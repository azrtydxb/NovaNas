package reconciler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// keycloakMock simulates just enough of the Keycloak admin REST API to
// exercise the Ensure* paths.
type keycloakMock struct {
	mux     *http.ServeMux
	realms  map[string]bool
	users   map[string]map[string]string // realm -> username -> id
	groups  map[string]map[string]string // realm -> name -> id
	nextID  int
	baseURL string
}

func newKeycloakMock(t *testing.T) *keycloakMock {
	t.Helper()
	m := &keycloakMock{
		realms: map[string]bool{"master": true},
		users:  map[string]map[string]string{},
		groups: map[string]map[string]string{},
		mux:    http.NewServeMux(),
	}
	// Token endpoint.
	m.mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"token_type":   "Bearer",
			"expires_in":   60,
		})
	})
	m.mux.HandleFunc("/admin/realms/", m.handleAdmin)
	return m
}

func (m *keycloakMock) id() string {
	m.nextID++
	return "id-" + time.Now().Format("150405") + "-" +
		string(rune('A'+m.nextID))
}

func (m *keycloakMock) handleAdmin(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/realms/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	realm := parts[0]
	// /admin/realms/<realm>
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			if !m.realms[realm] {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"realm":"` + realm + `","enabled":true}`))
			return
		case http.MethodPost, http.MethodPut:
			m.realms[realm] = true
			w.WriteHeader(http.StatusCreated)
			return
		case http.MethodDelete:
			delete(m.realms, realm)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	// Create realm (POST /admin/realms/)
	if len(parts) == 1 && parts[0] == "" && r.Method == http.MethodPost {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		name, _ := body["realm"].(string)
		m.realms[name] = true
		w.WriteHeader(http.StatusCreated)
		return
	}
	// /admin/realms/<realm>/users/<id> (PUT update, DELETE)
	if len(parts) == 3 && parts[1] == "users" {
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusNoContent)
			return
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	// /admin/realms/<realm>/users (GET list, POST create)
	if len(parts) == 2 && parts[1] == "users" {
		if _, ok := m.users[realm]; !ok {
			m.users[realm] = map[string]string{}
		}
		switch r.Method {
		case http.MethodGet:
			username := r.URL.Query().Get("username")
			w.Header().Set("Content-Type", "application/json")
			if id, ok := m.users[realm][username]; ok {
				_, _ = w.Write([]byte(`[{"id":"` + id + `","username":"` + username + `"}]`))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
			return
		case http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			username, _ := body["username"].(string)
			id := m.id()
			m.users[realm][username] = id
			w.Header().Set("Location", "/admin/realms/"+realm+"/users/"+id)
			w.WriteHeader(http.StatusCreated)
			return
		}
	}
	// /admin/realms/<realm>/groups
	if len(parts) == 2 && parts[1] == "groups" {
		if _, ok := m.groups[realm]; !ok {
			m.groups[realm] = map[string]string{}
		}
		switch r.Method {
		case http.MethodGet:
			name := r.URL.Query().Get("search")
			w.Header().Set("Content-Type", "application/json")
			if id, ok := m.groups[realm][name]; ok {
				_, _ = w.Write([]byte(`[{"id":"` + id + `","name":"` + name + `"}]`))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
			return
		case http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			name, _ := body["name"].(string)
			id := m.id()
			m.groups[realm][name] = id
			w.Header().Set("Location", "/admin/realms/"+realm+"/groups/"+id)
			w.WriteHeader(http.StatusCreated)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func TestGocloakClient_EnsureRealm_And_User_And_Group(t *testing.T) {
	mock := newKeycloakMock(t)
	srv := httptest.NewServer(mock.mux)
	defer srv.Close()
	mock.baseURL = srv.URL

	c, err := NewGocloakClient(GocloakConfig{
		BaseURL:      srv.URL,
		AdminRealm:   "master",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
	})
	if err != nil {
		t.Fatalf("NewGocloakClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.EnsureRealm(ctx, KeycloakRealmConfig{Name: "novanas", Enabled: true}); err != nil {
		t.Fatalf("EnsureRealm (new): %v", err)
	}
	if err := c.EnsureRealm(ctx, KeycloakRealmConfig{Name: "novanas", Enabled: true}); err != nil {
		t.Fatalf("EnsureRealm (existing): %v", err)
	}

	id, err := c.EnsureUser(ctx, KeycloakUser{Realm: "novanas", Username: "alice", Enabled: true})
	if err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	if id == "" {
		t.Fatalf("EnsureUser returned empty id")
	}
	// Second call should be idempotent (update path).
	if _, err := c.EnsureUser(ctx, KeycloakUser{Realm: "novanas", Username: "alice", Enabled: true}); err != nil {
		t.Fatalf("EnsureUser idempotent: %v", err)
	}

	gid, err := c.EnsureGroup(ctx, KeycloakGroup{Realm: "novanas", Name: "admins"})
	if err != nil {
		t.Fatalf("EnsureGroup: %v", err)
	}
	if gid == "" {
		t.Fatalf("EnsureGroup returned empty id")
	}
}
