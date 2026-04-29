package plugins

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// fakeKeycloak is a tiny in-memory stand-in for a Keycloak realm's
// admin REST API. Supports just the endpoints the OIDC + Permission
// provisioners touch.
type fakeKeycloak struct {
	mu sync.Mutex
	// keyed by Keycloak-internal UUID
	clients      map[string]*kcClient
	clientUUIDs  []string
	secrets      map[string]string // uuid -> rotated secret
	saUsers      map[string]string // clientUUID -> user UUID
	users        map[string]map[string]bool // userID -> set(roleNames)
	realmRoles   map[string]*kcRole

	// counters
	createCalls int
	rotateCalls int
	deleteCalls int

	server *httptest.Server
}

func newFakeKeycloak(t *testing.T) *fakeKeycloak {
	t.Helper()
	f := &fakeKeycloak{
		clients:    map[string]*kcClient{},
		secrets:    map[string]string{},
		saUsers:    map[string]string{},
		users:      map[string]map[string]bool{},
		realmRoles: map[string]*kcRole{},
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

// adminDoer adapts the fake into a KeycloakAdminDoer using the
// httptest server. We bypass token/auth — the fake serves anonymously.
type adminDoer struct{ baseURL string }

func (a *adminDoer) Do(ctx context.Context, method, relPath string, q url.Values, body io.Reader) (*http.Response, error) {
	u := a.baseURL + relPath
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return http.DefaultClient.Do(req)
}

func (f *fakeKeycloak) doer() *adminDoer { return &adminDoer{baseURL: f.server.URL} }

func (f *fakeKeycloak) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := r.URL.Path
	switch {
	case path == "/clients" && r.Method == http.MethodGet:
		want := r.URL.Query().Get("clientId")
		var out []kcClient
		for _, uuid := range f.clientUUIDs {
			c := f.clients[uuid]
			if want == "" || c.ClientID == want {
				cc := *c
				cc.ID = uuid
				out = append(out, cc)
			}
		}
		writeJSON(w, http.StatusOK, out)
	case path == "/clients" && r.Method == http.MethodPost:
		f.createCalls++
		var rep kcClient
		_ = json.NewDecoder(r.Body).Decode(&rep)
		uuid := "uuid-" + rep.ClientID
		stored := rep
		stored.ID = uuid
		f.clients[uuid] = &stored
		f.clientUUIDs = append(f.clientUUIDs, uuid)
		f.secrets[uuid] = "initial-secret"
		// Auto-create the SA user when serviceAccountsEnabled.
		if rep.ServiceAccountsEnabled {
			userID := "sa-user-" + rep.ClientID
			f.saUsers[uuid] = userID
			f.users[userID] = map[string]bool{}
		}
		w.WriteHeader(http.StatusCreated)
	case strings.HasPrefix(path, "/clients/") && strings.HasSuffix(path, "/client-secret") && r.Method == http.MethodPost:
		f.rotateCalls++
		uuid := strings.TrimSuffix(strings.TrimPrefix(path, "/clients/"), "/client-secret")
		f.secrets[uuid] = "rotated-secret"
		w.WriteHeader(http.StatusOK)
	case strings.HasPrefix(path, "/clients/") && strings.HasSuffix(path, "/client-secret") && r.Method == http.MethodGet:
		uuid := strings.TrimSuffix(strings.TrimPrefix(path, "/clients/"), "/client-secret")
		writeJSON(w, http.StatusOK, kcSecret{Value: f.secrets[uuid]})
	case strings.HasPrefix(path, "/clients/") && strings.HasSuffix(path, "/service-account-user") && r.Method == http.MethodGet:
		uuid := strings.TrimSuffix(strings.TrimPrefix(path, "/clients/"), "/service-account-user")
		userID, ok := f.saUsers[uuid]
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, kcUser{ID: userID})
	case strings.HasPrefix(path, "/clients/") && r.Method == http.MethodDelete:
		f.deleteCalls++
		uuid := strings.TrimPrefix(path, "/clients/")
		delete(f.clients, uuid)
		filtered := f.clientUUIDs[:0]
		for _, u := range f.clientUUIDs {
			if u != uuid {
				filtered = append(filtered, u)
			}
		}
		f.clientUUIDs = filtered
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(path, "/roles/") && r.Method == http.MethodGet:
		name := strings.TrimPrefix(path, "/roles/")
		role, ok := f.realmRoles[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, role)
	case strings.HasPrefix(path, "/users/") && strings.HasSuffix(path, "/role-mappings/realm") && r.Method == http.MethodGet:
		userID := strings.TrimSuffix(strings.TrimPrefix(path, "/users/"), "/role-mappings/realm")
		var out []kcRole
		for name := range f.users[userID] {
			if r := f.realmRoles[name]; r != nil {
				out = append(out, *r)
			}
		}
		writeJSON(w, http.StatusOK, out)
	case strings.HasPrefix(path, "/users/") && strings.HasSuffix(path, "/role-mappings/realm") && r.Method == http.MethodPost:
		userID := strings.TrimSuffix(strings.TrimPrefix(path, "/users/"), "/role-mappings/realm")
		var roles []kcRole
		_ = json.NewDecoder(r.Body).Decode(&roles)
		if f.users[userID] == nil {
			f.users[userID] = map[string]bool{}
		}
		for _, role := range roles {
			f.users[userID][role.Name] = true
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(path, "/users/") && strings.HasSuffix(path, "/role-mappings/realm") && r.Method == http.MethodDelete:
		userID := strings.TrimSuffix(strings.TrimPrefix(path, "/users/"), "/role-mappings/realm")
		var roles []kcRole
		_ = json.NewDecoder(r.Body).Decode(&roles)
		for _, role := range roles {
			delete(f.users[userID], role.Name)
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// memSecrets is an in-memory secrets.Manager-shaped fake. It would be
// circular to import the real secrets package, so we keep this local.
type memSecrets struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newMemSecrets() *memSecrets { return &memSecrets{m: map[string][]byte{}} }

func (s *memSecrets) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	if !ok {
		return nil, errSecretMissing
	}
	return append([]byte(nil), v...), nil
}
func (s *memSecrets) Set(_ context.Context, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = append([]byte(nil), value...)
	return nil
}
func (s *memSecrets) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}
func (s *memSecrets) List(_ context.Context, prefix string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for k := range s.m {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}
func (s *memSecrets) Backend() string { return "mem" }

type secretMissingErr struct{}

func (secretMissingErr) Error() string { return "not found" }

var errSecretMissing = secretMissingErr{}

func TestOIDCProvisioner_CreateAndDelete(t *testing.T) {
	fk := newFakeKeycloak(t)
	sec := newMemSecrets()
	p := &OIDCClientProvisioner{Admin: fk.doer(), Secrets: sec}

	id, err := p.Provision(context.Background(), "rustfs", OIDCClientNeed{ClientID: "rustfs"})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if id != "oidcclient:rustfs/rustfs" {
		t.Errorf("id=%q", id)
	}
	if fk.createCalls != 1 || fk.rotateCalls != 1 {
		t.Errorf("create=%d rotate=%d", fk.createCalls, fk.rotateCalls)
	}
	got, err := sec.Get(context.Background(), "nova/plugins/rustfs/oidc-client-secret")
	if err != nil {
		t.Fatalf("secret missing: %v", err)
	}
	if string(got) != "rotated-secret" {
		t.Errorf("secret=%q", got)
	}

	// Idempotent re-provision: client exists, but we still rotate — that's
	// the documented behaviour.
	if _, err := p.Provision(context.Background(), "rustfs", OIDCClientNeed{ClientID: "rustfs"}); err != nil {
		t.Fatalf("re-provision: %v", err)
	}
	if fk.createCalls != 1 {
		t.Errorf("create called again on existing client: %d", fk.createCalls)
	}

	if err := p.Unprovision(context.Background(), "rustfs", id); err != nil {
		t.Fatalf("unprovision: %v", err)
	}
	if fk.deleteCalls != 1 {
		t.Errorf("delete=%d", fk.deleteCalls)
	}
	if _, err := sec.Get(context.Background(), "nova/plugins/rustfs/oidc-client-secret"); err == nil {
		t.Error("secret should be gone")
	}
}

func TestPermissionProvisioner_BindAndUnbind(t *testing.T) {
	fk := newFakeKeycloak(t)
	// Pre-create the OIDC client + service account.
	op := &OIDCClientProvisioner{Admin: fk.doer(), Secrets: newMemSecrets()}
	if _, err := op.Provision(context.Background(), "rustfs", OIDCClientNeed{ClientID: "rustfs"}); err != nil {
		t.Fatal(err)
	}
	// Pre-define the realm role.
	fk.realmRoles["rustfs-admin"] = &kcRole{ID: "role-rustfs-admin", Name: "rustfs-admin"}

	pp := &PermissionProvisioner{Admin: fk.doer()}
	id, err := pp.Provision(context.Background(), "rustfs", PermissionNeed{Role: "rustfs-admin"})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if id != "permission:rustfs/rustfs-admin" {
		t.Errorf("id=%q", id)
	}
	if !fk.users["sa-user-rustfs"]["rustfs-admin"] {
		t.Error("role not bound")
	}
	// Idempotent re-provision: still succeeds, no extra binding.
	if _, err := pp.Provision(context.Background(), "rustfs", PermissionNeed{Role: "rustfs-admin"}); err != nil {
		t.Fatal(err)
	}

	if err := pp.Unprovision(context.Background(), "rustfs", id); err != nil {
		t.Fatalf("unprovision: %v", err)
	}
	if fk.users["sa-user-rustfs"]["rustfs-admin"] {
		t.Error("role still bound")
	}
}

func TestPermissionProvisioner_MissingRole(t *testing.T) {
	fk := newFakeKeycloak(t)
	op := &OIDCClientProvisioner{Admin: fk.doer(), Secrets: newMemSecrets()}
	if _, err := op.Provision(context.Background(), "rustfs", OIDCClientNeed{ClientID: "rustfs"}); err != nil {
		t.Fatal(err)
	}
	pp := &PermissionProvisioner{Admin: fk.doer()}
	if _, err := pp.Provision(context.Background(), "rustfs", PermissionNeed{Role: "no-such-role"}); err == nil {
		t.Fatal("expected error")
	}
}
