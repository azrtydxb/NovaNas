package reconciler

import "context"

// KeycloakUser is the minimal shape controllers pass to KeycloakClient.
type KeycloakUser struct {
	Realm     string
	Username  string
	Email     string
	FirstName string
	LastName  string
	Groups    []string
	Enabled   bool
}

// KeycloakGroup is the minimal shape controllers pass for group sync.
type KeycloakGroup struct {
	Realm     string
	Name      string
	ParentID  string
	Members   []string
}

// KeycloakRealmConfig is the minimal realm-level payload.
type KeycloakRealmConfig struct {
	Name        string
	DisplayName string
	Enabled     bool
	// RawJSON is an optional realm-representation JSON blob that takes
	// precedence over individual fields when non-nil.
	RawJSON []byte
}

// KeycloakClient abstracts the subset of Keycloak Admin API operations the
// NovaNas controllers need. Real implementations wrap the Keycloak REST
// client; tests inject NoopKeycloakClient.
type KeycloakClient interface {
	EnsureRealm(ctx context.Context, realm KeycloakRealmConfig) error
	DeleteRealm(ctx context.Context, realm string) error

	EnsureUser(ctx context.Context, user KeycloakUser) (userID string, err error)
	DeleteUser(ctx context.Context, realm, username string) error

	EnsureGroup(ctx context.Context, group KeycloakGroup) (groupID string, err error)
	DeleteGroup(ctx context.Context, realm, name string) error
}

// NoopKeycloakClient is the default fallback used when no Keycloak wiring
// is configured. All methods return success without doing anything, which
// lets controllers exercise their happy path in dev/test environments
// without a real identity provider.
type NoopKeycloakClient struct{}

// EnsureRealm is a no-op.
func (NoopKeycloakClient) EnsureRealm(_ context.Context, _ KeycloakRealmConfig) error { return nil }

// DeleteRealm is a no-op.
func (NoopKeycloakClient) DeleteRealm(_ context.Context, _ string) error { return nil }

// EnsureUser returns a deterministic placeholder ID.
func (NoopKeycloakClient) EnsureUser(_ context.Context, u KeycloakUser) (string, error) {
	return "noop-user-" + u.Username, nil
}

// DeleteUser is a no-op.
func (NoopKeycloakClient) DeleteUser(_ context.Context, _ string, _ string) error { return nil }

// EnsureGroup returns a deterministic placeholder ID.
func (NoopKeycloakClient) EnsureGroup(_ context.Context, g KeycloakGroup) (string, error) {
	return "noop-group-" + g.Name, nil
}

// DeleteGroup is a no-op.
func (NoopKeycloakClient) DeleteGroup(_ context.Context, _ string, _ string) error { return nil }
