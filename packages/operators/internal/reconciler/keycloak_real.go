// Package reconciler — gocloak-backed real implementation of KeycloakClient.
//
// This file is the production implementation injected at main.go wire-up
// time. The NoopKeycloakClient remains available as the test-time default.
package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Nerzal/gocloak/v13"
)

// GocloakConfig configures a GocloakClient.
type GocloakConfig struct {
	// BaseURL is the Keycloak base URL (e.g. "https://keycloak.example.com").
	BaseURL string
	// AdminRealm is the realm hosting the admin service-account client
	// (typically "master").
	AdminRealm string
	// ClientID is the admin service-account client id.
	ClientID string
	// ClientSecret is the admin service-account client secret.
	ClientSecret string
	// TokenTTL is a soft refresh window; tokens are refreshed this long
	// before their real expiry to avoid mid-request expiry.
	TokenTTL time.Duration
}

// GocloakClient is the real KeycloakClient implementation backed by the
// widely-used github.com/Nerzal/gocloak/v13 admin SDK.
//
// It authenticates with the Keycloak admin REST API using the OAuth2
// client-credentials flow and caches the resulting access token.
// The token is refreshed on-demand when it is close to expiry.
type GocloakClient struct {
	cfg GocloakConfig
	sdk *gocloak.GoCloak

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// NewGocloakClient constructs a GocloakClient and performs an initial
// admin login to validate credentials. It returns an error if the initial
// login fails so main.go can fall back to NoopKeycloakClient with a loud
// warning rather than silently keep a broken client.
func NewGocloakClient(cfg GocloakConfig) (*GocloakClient, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("gocloak: BaseURL is required")
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("gocloak: ClientID and ClientSecret are required")
	}
	if cfg.AdminRealm == "" {
		cfg.AdminRealm = "master"
	}
	if cfg.TokenTTL == 0 {
		cfg.TokenTTL = 30 * time.Second
	}
	c := &GocloakClient{
		cfg: cfg,
		sdk: gocloak.NewClient(cfg.BaseURL),
	}
	// Validate credentials up front so main.go can detect misconfig early.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.accessToken(ctx); err != nil {
		return nil, fmt.Errorf("gocloak: initial login: %w", err)
	}
	return c, nil
}

// accessToken returns a valid admin bearer token, refreshing if needed.
func (c *GocloakClient) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Until(c.expiresAt) > c.cfg.TokenTTL {
		return c.token, nil
	}
	tok, err := c.sdk.LoginClient(ctx, c.cfg.ClientID, c.cfg.ClientSecret, c.cfg.AdminRealm)
	if err != nil {
		return "", fmt.Errorf("login client: %w", err)
	}
	c.token = tok.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return c.token, nil
}

// EnsureRealm creates or updates a realm representation in Keycloak.
func (c *GocloakClient) EnsureRealm(ctx context.Context, cfg KeycloakRealmConfig) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}

	// Build realm representation. When RawJSON is supplied it takes precedence.
	var realm gocloak.RealmRepresentation
	if len(cfg.RawJSON) > 0 {
		if err := json.Unmarshal(cfg.RawJSON, &realm); err != nil {
			return fmt.Errorf("keycloak: parse realm RawJSON: %w", err)
		}
	}
	name := cfg.Name
	realm.Realm = &name
	enabled := cfg.Enabled
	realm.Enabled = &enabled
	if cfg.DisplayName != "" {
		disp := cfg.DisplayName
		realm.DisplayName = &disp
	}

	// Try GET to decide create vs update. gocloak returns an error containing
	// 404 when the realm does not exist.
	existing, gerr := c.sdk.GetRealm(ctx, token, cfg.Name)
	if gerr != nil || existing == nil {
		if _, cerr := c.sdk.CreateRealm(ctx, token, realm); cerr != nil {
			return fmt.Errorf("keycloak: create realm %q: %w", cfg.Name, cerr)
		}
		return nil
	}
	if uerr := c.sdk.UpdateRealm(ctx, token, realm); uerr != nil {
		return fmt.Errorf("keycloak: update realm %q: %w", cfg.Name, uerr)
	}
	return nil
}

// DeleteRealm removes a realm. Missing realms are treated as success.
func (c *GocloakClient) DeleteRealm(ctx context.Context, name string) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	if err := c.sdk.DeleteRealm(ctx, token, name); err != nil {
		// Treat 404 as idempotent success.
		if apiErr, ok := err.(*gocloak.APIError); ok && apiErr.Code == 404 {
			return nil
		}
		return fmt.Errorf("keycloak: delete realm %q: %w", name, err)
	}
	return nil
}

// EnsureUser upserts a user in the target realm by username and returns
// the resulting id.
func (c *GocloakClient) EnsureUser(ctx context.Context, u KeycloakUser) (string, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return "", err
	}
	username := u.Username
	email := u.Email
	enabled := u.Enabled
	first := u.FirstName
	last := u.LastName

	// Look up existing user by username (exact match).
	params := gocloak.GetUsersParams{Username: &username, Exact: boolPtr(true)}
	users, gerr := c.sdk.GetUsers(ctx, token, u.Realm, params)
	if gerr != nil {
		return "", fmt.Errorf("keycloak: get users: %w", gerr)
	}
	if len(users) > 0 && users[0].ID != nil {
		id := *users[0].ID
		// Update fields.
		users[0].Email = &email
		users[0].Enabled = &enabled
		users[0].FirstName = &first
		users[0].LastName = &last
		if uerr := c.sdk.UpdateUser(ctx, token, u.Realm, *users[0]); uerr != nil {
			return "", fmt.Errorf("keycloak: update user %q: %w", username, uerr)
		}
		return id, nil
	}

	rep := gocloak.User{
		Username:  &username,
		Email:     &email,
		FirstName: &first,
		LastName:  &last,
		Enabled:   &enabled,
	}
	id, cerr := c.sdk.CreateUser(ctx, token, u.Realm, rep)
	if cerr != nil {
		return "", fmt.Errorf("keycloak: create user %q: %w", username, cerr)
	}
	return id, nil
}

// DeleteUser removes a user by username. Missing users succeed.
func (c *GocloakClient) DeleteUser(ctx context.Context, realm, username string) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	params := gocloak.GetUsersParams{Username: &username, Exact: boolPtr(true)}
	users, gerr := c.sdk.GetUsers(ctx, token, realm, params)
	if gerr != nil {
		return fmt.Errorf("keycloak: get users: %w", gerr)
	}
	if len(users) == 0 || users[0].ID == nil {
		return nil
	}
	if err := c.sdk.DeleteUser(ctx, token, realm, *users[0].ID); err != nil {
		return fmt.Errorf("keycloak: delete user %q: %w", username, err)
	}
	return nil
}

// EnsureGroup upserts a top-level group by name and returns its id.
func (c *GocloakClient) EnsureGroup(ctx context.Context, g KeycloakGroup) (string, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return "", err
	}
	name := g.Name

	params := gocloak.GetGroupsParams{Search: &name}
	groups, gerr := c.sdk.GetGroups(ctx, token, g.Realm, params)
	if gerr != nil {
		return "", fmt.Errorf("keycloak: get groups: %w", gerr)
	}
	for _, gr := range groups {
		if gr != nil && gr.Name != nil && *gr.Name == name && gr.ID != nil {
			return *gr.ID, nil
		}
	}
	rep := gocloak.Group{Name: &name}
	id, cerr := c.sdk.CreateGroup(ctx, token, g.Realm, rep)
	if cerr != nil {
		return "", fmt.Errorf("keycloak: create group %q: %w", name, cerr)
	}
	return id, nil
}

// DeleteGroup deletes a group by name. Missing groups succeed.
func (c *GocloakClient) DeleteGroup(ctx context.Context, realm, name string) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	params := gocloak.GetGroupsParams{Search: &name}
	groups, gerr := c.sdk.GetGroups(ctx, token, realm, params)
	if gerr != nil {
		return fmt.Errorf("keycloak: get groups: %w", gerr)
	}
	for _, gr := range groups {
		if gr != nil && gr.Name != nil && *gr.Name == name && gr.ID != nil {
			if err := c.sdk.DeleteGroup(ctx, token, realm, *gr.ID); err != nil {
				return fmt.Errorf("keycloak: delete group %q: %w", name, err)
			}
			return nil
		}
	}
	return nil
}

func boolPtr(b bool) *bool { return &b }

// Ensure GocloakClient satisfies KeycloakClient.
var _ KeycloakClient = (*GocloakClient)(nil)
