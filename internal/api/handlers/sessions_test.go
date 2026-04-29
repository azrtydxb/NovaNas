package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/auth"
)

// fakeKeycloak is a minimal Keycloak admin + token fake. It serves both
// the OIDC token endpoint (POST /token) and arbitrary admin endpoints
// rooted at /admin/realms/test.
type fakeKeycloak struct {
	*httptest.Server
	gotPath   string
	gotMethod string
	respBody  string
	respCode  int
	sessions  string // JSON to return on /sessions list
}

func newFakeKeycloak(t *testing.T) *fakeKeycloak {
	t.Helper()
	f := &fakeKeycloak{respCode: 200, respBody: `[]`}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fake","expires_in":300}`))
	})
	mux.HandleFunc("/admin/realms/test/", func(w http.ResponseWriter, r *http.Request) {
		f.gotMethod = r.Method
		f.gotPath = strings.TrimPrefix(r.URL.Path, "/admin/realms/test")
		w.Header().Set("Content-Type", "application/json")
		// /sessions list returns the configured sessions JSON; everything
		// else returns respBody.
		if strings.HasSuffix(f.gotPath, "/sessions") && r.Method == "GET" {
			w.WriteHeader(200)
			body := f.sessions
			if body == "" {
				body = "[]"
			}
			_, _ = w.Write([]byte(body))
			return
		}
		w.WriteHeader(f.respCode)
		_, _ = w.Write([]byte(f.respBody))
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func newSessionsTestRouter(t *testing.T, f *fakeKeycloak, identity *auth.Identity) *chi.Mux {
	h := &SessionsHandler{
		Admin: &KeycloakAdminClient{
			AdminURL:     f.URL + "/admin/realms/test",
			TokenURL:     f.URL + "/token",
			ClientID:     "nova-api-admin",
			ClientSecret: "s",
			HTTP:         f.Client(),
		},
		RoleMap: auth.DefaultRoleMap,
	}
	r := chi.NewRouter()
	if identity != nil {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := auth.WithIdentity(req.Context(), identity)
				next.ServeHTTP(w, req.WithContext(ctx))
			})
		})
	}
	r.Get("/api/v1/auth/sessions", h.ListOwnSessions)
	r.Delete("/api/v1/auth/sessions/{id}", h.RevokeOwnSession)
	r.Get("/api/v1/auth/users/{id}/sessions", h.ListUserSessions)
	r.Delete("/api/v1/auth/users/{id}/sessions", h.RevokeUserSessions)
	r.Get("/api/v1/auth/login-history", h.ListOwnLoginHistory)
	r.Get("/api/v1/auth/users/{id}/login-history", h.ListUserLoginHistory)
	return r
}

func TestSessionsListOwn(t *testing.T) {
	f := newFakeKeycloak(t)
	f.sessions = `[{"id":"s1"}]`
	r := newSessionsTestRouter(t, f, &auth.Identity{Subject: "user-42"})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/auth/sessions", nil))
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.gotPath != "/users/user-42/sessions" {
		t.Errorf("upstream path=%s", f.gotPath)
	}
}

func TestSessionsRevokeOwnRequiresOwnership(t *testing.T) {
	f := newFakeKeycloak(t)
	f.sessions = `[{"id":"s1"}]`
	r := newSessionsTestRouter(t, f, &auth.Identity{Subject: "user-42"})

	// Owned session — should pass.
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("DELETE", "/api/v1/auth/sessions/s1", nil))
	if rr.Code != 200 {
		t.Fatalf("owned: status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Foreign session — should 404.
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("DELETE", "/api/v1/auth/sessions/foreign", nil))
	if rr.Code != 404 {
		t.Errorf("foreign: status=%d", rr.Code)
	}
}

func TestSessionsAdminListRequiresAdminPerm(t *testing.T) {
	f := newFakeKeycloak(t)
	// Viewer should be rejected.
	r := newSessionsTestRouter(t, f, &auth.Identity{Subject: "viewer", Roles: []string{"nova-viewer"}})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/auth/users/abc/sessions", nil))
	if rr.Code != 403 {
		t.Errorf("viewer status=%d body=%s", rr.Code, rr.Body.String())
	}
	// Admin should pass.
	r = newSessionsTestRouter(t, f, &auth.Identity{Subject: "admin", Roles: []string{"nova-admin"}})
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/auth/users/abc/sessions", nil))
	if rr.Code != 200 {
		t.Errorf("admin status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSessionsLoginHistoryAddsUserParam(t *testing.T) {
	f := newFakeKeycloak(t)
	f.respBody = `[]`
	captured := ""
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"x","expires_in":300}`))
	})
	mux.HandleFunc("/admin/realms/test/events", func(w http.ResponseWriter, r *http.Request) {
		captured = r.URL.RawQuery
		_, _ = w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	h := &SessionsHandler{
		Admin: &KeycloakAdminClient{
			AdminURL: srv.URL + "/admin/realms/test",
			TokenURL: srv.URL + "/token",
			ClientID: "x", ClientSecret: "y",
			HTTP: srv.Client(),
		},
		RoleMap: auth.DefaultRoleMap,
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := auth.WithIdentity(req.Context(), &auth.Identity{Subject: "u-1"})
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Get("/api/v1/auth/login-history", h.ListOwnLoginHistory)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/auth/login-history?max=10", nil))
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(captured, "user=u-1") || !strings.Contains(captured, "type=LOGIN") {
		t.Errorf("query=%s", captured)
	}
}

func TestSessionsUnconfigured(t *testing.T) {
	h := &SessionsHandler{}
	r := chi.NewRouter()
	r.Get("/api/v1/auth/sessions", h.ListOwnSessions)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/auth/sessions", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status=%d", rr.Code)
	}
}
