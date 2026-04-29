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
)

// PermissionProvisioner binds a Keycloak realm role to the
// service-account user of the plugin's OIDC client. Idempotent: if the
// binding already exists, Provision is a no-op.
type PermissionProvisioner struct {
	Admin  KeycloakAdminDoer
	Logger *slog.Logger

	// ClientIDFor is consulted to find the plugin's OIDC client id (the
	// human clientId, not Keycloak's UUID). Production wires this to
	// look up the most recent OIDC clientId provisioned for the plugin.
	// If nil, the provisioner falls back to using the plugin name.
	ClientIDFor func(plugin string) string
}

func permissionResourceID(plugin, role string) string {
	return fmt.Sprintf("permission:%s/%s", plugin, role)
}

func parsePermissionResourceID(id string) (plugin, role string, ok bool) {
	if !strings.HasPrefix(id, "permission:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(id, "permission:")
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return "", "", false
	}
	return rest[:slash], rest[slash+1:], true
}

type kcRole struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContainerID   string `json:"containerId,omitempty"`
	ClientRole    bool   `json:"clientRole,omitempty"`
	Composite     bool   `json:"composite,omitempty"`
}

type kcUser struct {
	ID string `json:"id"`
}

func (p *PermissionProvisioner) clientIDFor(plugin string) string {
	if p.ClientIDFor != nil {
		if v := p.ClientIDFor(plugin); v != "" {
			return v
		}
	}
	return plugin
}

// Provision assigns realm role n.Role to the SA user of the plugin's
// OIDC client.
func (p *PermissionProvisioner) Provision(ctx context.Context, plugin string, n PermissionNeed) (string, error) {
	if p.Admin == nil {
		return "", errors.New("plugins: PermissionProvisioner.Admin is nil")
	}
	if n.Role == "" {
		return "", fmt.Errorf("plugins: permission need: role required")
	}
	role, err := p.findRealmRole(ctx, n.Role)
	if err != nil {
		return "", err
	}
	if role == nil {
		return "", fmt.Errorf("plugins: realm role %q not found", n.Role)
	}
	userID, err := p.serviceAccountUserID(ctx, p.clientIDFor(plugin))
	if err != nil {
		return "", err
	}

	// Idempotency: skip the POST if the user already has this role.
	has, err := p.userHasRealmRole(ctx, userID, role.Name)
	if err != nil {
		return "", err
	}
	if !has {
		body, _ := json.Marshal([]kcRole{*role})
		resp, err := p.Admin.Do(ctx, http.MethodPost,
			"/users/"+url.PathEscape(userID)+"/role-mappings/realm",
			nil, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("plugins: keycloak bind role: %w", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return "", fmt.Errorf("plugins: keycloak bind role: %d", resp.StatusCode)
		}
	}
	if p.Logger != nil {
		p.Logger.Info("plugins: permission bound", "plugin", plugin, "role", n.Role)
	}
	return permissionResourceID(plugin, n.Role), nil
}

// Unprovision removes the role binding from the SA user.
func (p *PermissionProvisioner) Unprovision(ctx context.Context, plugin, resourceID string) error {
	if p.Admin == nil {
		return errors.New("plugins: PermissionProvisioner.Admin is nil")
	}
	_, role, ok := parsePermissionResourceID(resourceID)
	if !ok {
		return fmt.Errorf("plugins: bad permission resource id %q", resourceID)
	}
	roleRep, err := p.findRealmRole(ctx, role)
	if err != nil || roleRep == nil {
		return nil
	}
	userID, err := p.serviceAccountUserID(ctx, p.clientIDFor(plugin))
	if err != nil {
		// SA user is gone with the OIDC client — nothing to unbind.
		return nil
	}
	body, _ := json.Marshal([]kcRole{*roleRep})
	resp, err := p.Admin.Do(ctx, http.MethodDelete,
		"/users/"+url.PathEscape(userID)+"/role-mappings/realm",
		nil, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("plugins: keycloak unbind role: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("plugins: keycloak unbind role: %d", resp.StatusCode)
	}
	if p.Logger != nil {
		p.Logger.Info("plugins: permission unbound", "plugin", plugin, "role", role)
	}
	return nil
}

func (p *PermissionProvisioner) findRealmRole(ctx context.Context, name string) (*kcRole, error) {
	resp, err := p.Admin.Do(ctx, http.MethodGet, "/roles/"+url.PathEscape(name), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("plugins: keycloak get role: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("plugins: keycloak get role: %d", resp.StatusCode)
	}
	var role kcRole
	if err := json.NewDecoder(resp.Body).Decode(&role); err != nil {
		return nil, fmt.Errorf("plugins: keycloak get role decode: %w", err)
	}
	return &role, nil
}

func (p *PermissionProvisioner) serviceAccountUserID(ctx context.Context, clientID string) (string, error) {
	// Find the client UUID first.
	q := url.Values{"clientId": []string{clientID}}
	listResp, err := p.Admin.Do(ctx, http.MethodGet, "/clients", q, nil)
	if err != nil {
		return "", fmt.Errorf("plugins: keycloak list clients: %w", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode/100 != 2 {
		return "", fmt.Errorf("plugins: keycloak list clients: %d", listResp.StatusCode)
	}
	var clients []kcClient
	if err := json.NewDecoder(listResp.Body).Decode(&clients); err != nil {
		return "", fmt.Errorf("plugins: keycloak list clients decode: %w", err)
	}
	var uuid string
	for _, c := range clients {
		if c.ClientID == clientID {
			uuid = c.ID
			break
		}
	}
	if uuid == "" {
		return "", fmt.Errorf("plugins: no oidc client %q for service account", clientID)
	}
	saResp, err := p.Admin.Do(ctx, http.MethodGet, "/clients/"+url.PathEscape(uuid)+"/service-account-user", nil, nil)
	if err != nil {
		return "", fmt.Errorf("plugins: keycloak sa user: %w", err)
	}
	defer saResp.Body.Close()
	if saResp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(saResp.Body)
		return "", fmt.Errorf("plugins: keycloak sa user: %d %s", saResp.StatusCode, string(body))
	}
	var u kcUser
	if err := json.NewDecoder(saResp.Body).Decode(&u); err != nil {
		return "", fmt.Errorf("plugins: keycloak sa user decode: %w", err)
	}
	if u.ID == "" {
		return "", fmt.Errorf("plugins: keycloak sa user: empty id")
	}
	return u.ID, nil
}

func (p *PermissionProvisioner) userHasRealmRole(ctx context.Context, userID, role string) (bool, error) {
	resp, err := p.Admin.Do(ctx, http.MethodGet,
		"/users/"+url.PathEscape(userID)+"/role-mappings/realm",
		nil, nil)
	if err != nil {
		return false, fmt.Errorf("plugins: keycloak list role mappings: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return false, fmt.Errorf("plugins: keycloak list role mappings: %d", resp.StatusCode)
	}
	var existing []kcRole
	if err := json.NewDecoder(resp.Body).Decode(&existing); err != nil {
		return false, fmt.Errorf("plugins: keycloak list role mappings decode: %w", err)
	}
	for _, r := range existing {
		if r.Name == role {
			return true, nil
		}
	}
	return false, nil
}
