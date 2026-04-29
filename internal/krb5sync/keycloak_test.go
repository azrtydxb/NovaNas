package krb5sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeKC stands up an httptest.Server that mimics the Keycloak admin
// endpoints we use. It speaks the bare minimum: client_credentials token,
// list users, get user, list admin events.
type fakeKC struct {
	users  []KeycloakUser
	events []AdminEvent
}

func (f *fakeKC) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/novanas/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("grant_type") != "client_credentials" {
			http.Error(w, "bad grant", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"expires_in":   300,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/admin/realms/novanas/users", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Return everything on first page; pagination break occurs at len<100.
		if r.URL.Query().Get("first") != "0" {
			_ = json.NewEncoder(w).Encode([]KeycloakUser{})
			return
		}
		_ = json.NewEncoder(w).Encode(f.users)
	})
	mux.HandleFunc("/admin/realms/novanas/admin-events", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.events)
	})
	return mux
}

func TestKeycloakClientListUsers(t *testing.T) {
	f := &fakeKC{users: []KeycloakUser{
		{ID: "u1", Username: "alice", Enabled: true, Attributes: map[string][]string{TenantAttribute: {"acme"}}},
		{ID: "u2", Username: "bob", Enabled: true},
	}}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()

	kc, err := NewKeycloakClient(context.Background(), KeycloakConfig{
		BaseURL: srv.URL, Realm: "novanas",
		ClientID: "nova-krb5-sync", ClientSecret: "x",
	})
	if err != nil {
		t.Fatalf("NewKeycloakClient: %v", err)
	}
	users, err := kc.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("got %d users, want 2", len(users))
	}
	if users[0].Username != "alice" {
		t.Errorf("user[0]=%q, want alice", users[0].Username)
	}
	if got := users[0].Tenants(); len(got) != 1 || got[0] != "acme" {
		t.Errorf("alice tenants=%v, want [acme]", got)
	}
}

func TestKeycloakClientListAdminEvents(t *testing.T) {
	f := &fakeKC{events: []AdminEvent{
		{Time: 1700000000000, OperationType: "CREATE", ResourceType: "USER", ResourcePath: "users/u1"},
		{Time: 1700000050000, OperationType: "UPDATE", ResourceType: "USER", ResourcePath: "users/u2"},
	}}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()

	kc, err := NewKeycloakClient(context.Background(), KeycloakConfig{
		BaseURL: srv.URL, Realm: "novanas",
		ClientID: "nova-krb5-sync", ClientSecret: "x",
	})
	if err != nil {
		t.Fatalf("NewKeycloakClient: %v", err)
	}
	events, err := kc.ListAdminEvents(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("ListAdminEvents: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("got %d events, want 2", len(events))
	}
}
