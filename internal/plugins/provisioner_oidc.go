package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/novanas/nova-nas/internal/host/secrets"
)

// KeycloakAdminDoer is the narrow surface OIDCClientProvisioner needs
// from the Keycloak admin client. *handlers.KeycloakAdminClient
// satisfies it; tests provide an httptest-backed fake.
type KeycloakAdminDoer interface {
	Do(ctx context.Context, method, relPath string, q url.Values, body io.Reader) (*http.Response, error)
}

// OIDCClientProvisioner creates Keycloak clients for plugins. It mints
// a fresh client_secret, stashes it in the secrets backend at
// nova/plugins/<plugin>/oidc-client-secret, and returns a stable
// resource ID. On rollback it deletes the Keycloak client and the
// stored secret.
type OIDCClientProvisioner struct {
	Admin   KeycloakAdminDoer
	Secrets secrets.Manager
	Logger  *slog.Logger
}

func oidcResourceID(plugin, clientID string) string {
	return fmt.Sprintf("oidcclient:%s/%s", plugin, clientID)
}

// parseOIDCResourceID returns the plugin name and Keycloak clientId
// (the human one, NOT Keycloak's UUID) from a resource ID.
func parseOIDCResourceID(id string) (plugin, clientID string, ok bool) {
	if !strings.HasPrefix(id, "oidcclient:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(id, "oidcclient:")
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return "", "", false
	}
	return rest[:slash], rest[slash+1:], true
}

func oidcSecretPath(plugin string) string {
	return "nova/plugins/" + plugin + "/oidc-client-secret"
}

// kcClient is the JSON shape Keycloak's admin REST API uses for
// /clients. Only the fields we set / read are listed.
type kcClient struct {
	ID                      string   `json:"id,omitempty"`
	ClientID                string   `json:"clientId"`
	Enabled                 bool     `json:"enabled"`
	PublicClient            bool     `json:"publicClient"`
	ServiceAccountsEnabled  bool     `json:"serviceAccountsEnabled"`
	StandardFlowEnabled     bool     `json:"standardFlowEnabled"`
	DirectAccessGrantsOn    bool     `json:"directAccessGrantsEnabled"`
	RedirectURIs            []string `json:"redirectUris,omitempty"`
	WebOrigins              []string `json:"webOrigins,omitempty"`
	ProtocolMappers         []kcProtocolMapper `json:"protocolMappers,omitempty"`
}

type kcProtocolMapper struct {
	Name           string            `json:"name"`
	Protocol       string            `json:"protocol"`
	ProtocolMapper string            `json:"protocolMapper"`
	Config         map[string]string `json:"config"`
}

type kcSecret struct {
	Value string `json:"value"`
}

// Provision either creates or rotates the Keycloak client and stashes
// its secret. Returns the stable resource ID. The Keycloak-internal
// UUID is intentionally not surfaced — Permission rollback resolves it
// dynamically by listing /clients?clientId=...
func (p *OIDCClientProvisioner) Provision(ctx context.Context, plugin string, n OIDCClientNeed) (string, error) {
	if p.Admin == nil {
		return "", errors.New("plugins: OIDCClientProvisioner.Admin is nil")
	}
	if n.ClientID == "" {
		return "", fmt.Errorf("plugins: oidc need: clientId required")
	}
	uuid, err := p.findClientUUID(ctx, n.ClientID)
	if err != nil {
		return "", err
	}
	rep := kcClient{
		ClientID:               n.ClientID,
		Enabled:                true,
		PublicClient:           n.Public,
		ServiceAccountsEnabled: !n.Public,
		StandardFlowEnabled:    true,
		DirectAccessGrantsOn:   false,
		RedirectURIs:           n.Redirects,
		ProtocolMappers: []kcProtocolMapper{{
			Name:           "audience-" + n.ClientID,
			Protocol:       "openid-connect",
			ProtocolMapper: "oidc-audience-mapper",
			Config: map[string]string{
				"included.client.audience": n.ClientID,
				"id.token.claim":           "false",
				"access.token.claim":       "true",
			},
		}},
	}
	if uuid == "" {
		body, _ := json.Marshal(rep)
		resp, err := p.Admin.Do(ctx, http.MethodPost, "/clients", nil, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("plugins: keycloak create client: %w", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return "", fmt.Errorf("plugins: keycloak create client: %d", resp.StatusCode)
		}
		uuid, err = p.findClientUUID(ctx, n.ClientID)
		if err != nil || uuid == "" {
			return "", fmt.Errorf("plugins: keycloak create client: lookup post-create: %v", err)
		}
	}

	if !n.Public {
		// Rotate + read the secret. POST .../client-secret rotates, then GET
		// returns the current value.
		rotResp, err := p.Admin.Do(ctx, http.MethodPost, "/clients/"+url.PathEscape(uuid)+"/client-secret", nil, nil)
		if err != nil {
			return "", fmt.Errorf("plugins: keycloak rotate secret: %w", err)
		}
		_ = rotResp.Body.Close()
		if rotResp.StatusCode/100 != 2 {
			return "", fmt.Errorf("plugins: keycloak rotate secret: %d", rotResp.StatusCode)
		}
		getResp, err := p.Admin.Do(ctx, http.MethodGet, "/clients/"+url.PathEscape(uuid)+"/client-secret", nil, nil)
		if err != nil {
			return "", fmt.Errorf("plugins: keycloak get secret: %w", err)
		}
		defer getResp.Body.Close()
		if getResp.StatusCode/100 != 2 {
			return "", fmt.Errorf("plugins: keycloak get secret: %d", getResp.StatusCode)
		}
		var sec kcSecret
		if err := json.NewDecoder(getResp.Body).Decode(&sec); err != nil {
			return "", fmt.Errorf("plugins: keycloak get secret: %w", err)
		}
		if p.Secrets != nil && sec.Value != "" {
			if err := p.Secrets.Set(ctx, oidcSecretPath(plugin), []byte(sec.Value)); err != nil {
				return "", fmt.Errorf("plugins: store oidc secret: %w", err)
			}
		}
	}

	if p.Logger != nil {
		p.Logger.Info("plugins: oidc client ready", "plugin", plugin, "clientId", n.ClientID)
	}
	return oidcResourceID(plugin, n.ClientID), nil
}

// Unprovision deletes the Keycloak client and the escrowed secret.
func (p *OIDCClientProvisioner) Unprovision(ctx context.Context, plugin, resourceID string) error {
	if p.Admin == nil {
		return errors.New("plugins: OIDCClientProvisioner.Admin is nil")
	}
	_, clientID, ok := parseOIDCResourceID(resourceID)
	if !ok {
		return fmt.Errorf("plugins: bad oidc resource id %q", resourceID)
	}
	uuid, err := p.findClientUUID(ctx, clientID)
	if err != nil {
		return err
	}
	if uuid != "" {
		resp, err := p.Admin.Do(ctx, http.MethodDelete, "/clients/"+url.PathEscape(uuid), nil, nil)
		if err != nil {
			return fmt.Errorf("plugins: keycloak delete client: %w", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
			return fmt.Errorf("plugins: keycloak delete client: %d", resp.StatusCode)
		}
	}
	if p.Secrets != nil {
		_ = p.Secrets.Delete(ctx, oidcSecretPath(plugin))
	}
	if p.Logger != nil {
		p.Logger.Info("plugins: oidc client deleted", "plugin", plugin, "clientId", clientID)
	}
	return nil
}

// findClientUUID returns Keycloak's internal client UUID for the given
// human clientId, or "" if not present.
func (p *OIDCClientProvisioner) findClientUUID(ctx context.Context, clientID string) (string, error) {
	q := url.Values{"clientId": []string{clientID}}
	resp, err := p.Admin.Do(ctx, http.MethodGet, "/clients", q, nil)
	if err != nil {
		return "", fmt.Errorf("plugins: keycloak list clients: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("plugins: keycloak list clients: %d", resp.StatusCode)
	}
	var got []kcClient
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		return "", fmt.Errorf("plugins: keycloak list clients decode: %w", err)
	}
	for _, c := range got {
		if c.ClientID == clientID {
			return c.ID, nil
		}
	}
	return "", nil
}
