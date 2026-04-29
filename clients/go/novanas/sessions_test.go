package novanas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListOwnSessions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		if r.URL.Path != "/api/v1/auth/sessions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		writeJSON(t, w, 200, []Session{{ID: "s1", Username: "alice"}})
	}))
	defer srv.Close()
	got, err := newTestClient(t, srv).ListOwnSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "s1" {
		t.Errorf("got=%+v", got)
	}
}

func TestRevokeOwnSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/api/v1/auth/sessions/s1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	if err := newTestClient(t, srv).RevokeOwnSession(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
}

func TestListOwnLoginHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login-history" {
			t.Errorf("path=%s", r.URL.Path)
		}
		writeJSON(t, w, 200, []LoginEvent{{Type: "LOGIN", UserID: "u-1"}})
	}))
	defer srv.Close()
	got, err := newTestClient(t, srv).ListOwnLoginHistory(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Type != "LOGIN" {
		t.Errorf("got=%+v", got)
	}
}
