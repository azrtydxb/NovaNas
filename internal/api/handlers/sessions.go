// Package handlers — Keycloak admin pass-through for sessions and login
// history.
//
// nova-api authenticates the caller, then uses its OWN admin-API
// client_credentials token to call Keycloak. The caller's bearer token
// is never sent to the admin API. The "caller's own sessions" routes
// resolve the caller's `sub` claim from the Identity attached to the
// request context by the auth middleware.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/auth"
)

// KeycloakAdminClient is a minimal client for Keycloak's Admin REST API.
// It owns its own client_credentials token cache; do/Refresh are
// goroutine-safe.
type KeycloakAdminClient struct {
	// AdminURL is the realm-scoped admin base URL, e.g.
	// https://kc.example.com/admin/realms/novanas
	AdminURL string
	// TokenURL is the OIDC token endpoint, e.g.
	// https://kc.example.com/realms/novanas/protocol/openid-connect/token
	TokenURL string
	// ClientID + ClientSecret for client_credentials grant.
	ClientID     string
	ClientSecret string

	HTTP *http.Client

	mu         sync.Mutex
	tok        string
	tokExpires time.Time
}

func (c *KeycloakAdminClient) httpc() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// token returns a non-expired admin-API access token, fetching one via
// client_credentials when needed.
func (c *KeycloakAdminClient) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tok != "" && time.Now().Before(c.tokExpires.Add(-30*time.Second)) {
		return c.tok, nil
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpc().Do(req)
	if err != nil {
		return "", fmt.Errorf("keycloak token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("keycloak token: %d %s", resp.StatusCode, string(body))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("keycloak token decode: %w", err)
	}
	c.tok = tr.AccessToken
	if tr.ExpiresIn <= 0 {
		tr.ExpiresIn = 60
	}
	c.tokExpires = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return c.tok, nil
}

// Do builds an admin-API request, attaches the cached bearer, and
// returns the raw response. Caller must close resp.Body.
func (c *KeycloakAdminClient) Do(ctx context.Context, method, relPath string, q url.Values, body io.Reader) (*http.Response, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	u := strings.TrimRight(c.AdminURL, "/") + relPath
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpc().Do(req)
}

// SessionsHandler exposes /api/v1/auth/sessions and /auth/login-history.
type SessionsHandler struct {
	Logger  *slog.Logger
	Admin   *KeycloakAdminClient
	RoleMap auth.RoleMap
}

func (h *SessionsHandler) callerID(r *http.Request) string {
	if id, ok := auth.IdentityFromContext(r.Context()); ok && id != nil {
		return id.Subject
	}
	return ""
}

func (h *SessionsHandler) isAdmin(r *http.Request) bool {
	id, ok := auth.IdentityFromContext(r.Context())
	if !ok || id == nil {
		// When auth is disabled (dev), be permissive — there's no identity.
		return true
	}
	rm := h.RoleMap
	if rm == nil {
		rm = auth.DefaultRoleMap
	}
	return auth.IdentityHasPermission(rm, id, auth.PermSessionsAdmin)
}

func (h *SessionsHandler) ensureAdminWired(w http.ResponseWriter) bool {
	if h.Admin == nil || h.Admin.AdminURL == "" {
		middleware.WriteError(w, http.StatusServiceUnavailable, "keycloak_admin_unconfigured",
			"KEYCLOAK_ADMIN_CLIENT_ID is not configured")
		return false
	}
	return true
}

// proxy reads upstream, passes status+body through, normalizing 5xx into
// a 502 envelope when the body isn't JSON.
func (h *SessionsHandler) proxy(w http.ResponseWriter, resp *http.Response, err error, op string) {
	if err != nil {
		if h.Logger != nil {
			h.Logger.Warn("keycloak admin call", "op", op, "err", err)
		}
		middleware.WriteError(w, http.StatusBadGateway, "keycloak_unreachable", err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// ListOwnSessions handles GET /api/v1/auth/sessions.
func (h *SessionsHandler) ListOwnSessions(w http.ResponseWriter, r *http.Request) {
	if !h.ensureAdminWired(w) {
		return
	}
	uid := h.callerID(r)
	if uid == "" {
		middleware.WriteError(w, http.StatusForbidden, "no_identity", "caller identity unavailable")
		return
	}
	resp, err := h.Admin.Do(r.Context(), http.MethodGet, "/users/"+url.PathEscape(uid)+"/sessions", nil, nil)
	h.proxy(w, resp, err, "self.sessions.list")
}

// RevokeOwnSession handles DELETE /api/v1/auth/sessions/{id}.
func (h *SessionsHandler) RevokeOwnSession(w http.ResponseWriter, r *http.Request) {
	if !h.ensureAdminWired(w) {
		return
	}
	uid := h.callerID(r)
	if uid == "" {
		middleware.WriteError(w, http.StatusForbidden, "no_identity", "caller identity unavailable")
		return
	}
	sid := strings.TrimSpace(chi.URLParam(r, "id"))
	if sid == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", "session id is required")
		return
	}
	// First confirm this session belongs to the caller, then call the
	// generic /sessions/{id} delete endpoint.
	resp, err := h.Admin.Do(r.Context(), http.MethodGet, "/users/"+url.PathEscape(uid)+"/sessions", nil, nil)
	if err != nil {
		middleware.WriteError(w, http.StatusBadGateway, "keycloak_unreachable", err.Error())
		return
	}
	owns := false
	if resp.StatusCode == 200 {
		var sessions []map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&sessions)
		for _, s := range sessions {
			if v, _ := s["id"].(string); v == sid {
				owns = true
				break
			}
		}
	}
	resp.Body.Close()
	if !owns {
		middleware.WriteError(w, http.StatusNotFound, "not_found", "no session with that id for caller")
		return
	}
	delResp, err := h.Admin.Do(r.Context(), http.MethodDelete, "/sessions/"+url.PathEscape(sid), nil, nil)
	h.proxy(w, delResp, err, "self.sessions.revoke")
}

// ListUserSessions handles GET /api/v1/auth/users/{id}/sessions (admin).
func (h *SessionsHandler) ListUserSessions(w http.ResponseWriter, r *http.Request) {
	if !h.ensureAdminWired(w) {
		return
	}
	if !h.isAdmin(r) {
		middleware.WriteError(w, http.StatusForbidden, "forbidden", "requires nova:sessions:admin")
		return
	}
	uid := strings.TrimSpace(chi.URLParam(r, "id"))
	if uid == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", "user id is required")
		return
	}
	resp, err := h.Admin.Do(r.Context(), http.MethodGet, "/users/"+url.PathEscape(uid)+"/sessions", nil, nil)
	h.proxy(w, resp, err, "user.sessions.list")
}

// RevokeUserSessions handles DELETE /api/v1/auth/users/{id}/sessions
// (admin) — revokes ALL of the user's sessions.
func (h *SessionsHandler) RevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	if !h.ensureAdminWired(w) {
		return
	}
	if !h.isAdmin(r) {
		middleware.WriteError(w, http.StatusForbidden, "forbidden", "requires nova:sessions:admin")
		return
	}
	uid := strings.TrimSpace(chi.URLParam(r, "id"))
	if uid == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", "user id is required")
		return
	}
	resp, err := h.Admin.Do(r.Context(), http.MethodPost, "/users/"+url.PathEscape(uid)+"/logout", nil, nil)
	h.proxy(w, resp, err, "user.sessions.revoke_all")
}

// ListOwnLoginHistory handles GET /api/v1/auth/login-history.
func (h *SessionsHandler) ListOwnLoginHistory(w http.ResponseWriter, r *http.Request) {
	if !h.ensureAdminWired(w) {
		return
	}
	uid := h.callerID(r)
	if uid == "" {
		middleware.WriteError(w, http.StatusForbidden, "no_identity", "caller identity unavailable")
		return
	}
	q := url.Values{}
	for k, vs := range r.URL.Query() {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	q.Set("user", uid)
	if q.Get("type") == "" {
		q.Set("type", "LOGIN")
	}
	resp, err := h.Admin.Do(r.Context(), http.MethodGet, "/events", q, nil)
	h.proxy(w, resp, err, "self.login_history")
}

// ListUserLoginHistory handles GET /api/v1/auth/users/{id}/login-history
// (admin).
func (h *SessionsHandler) ListUserLoginHistory(w http.ResponseWriter, r *http.Request) {
	if !h.ensureAdminWired(w) {
		return
	}
	if !h.isAdmin(r) {
		middleware.WriteError(w, http.StatusForbidden, "forbidden", "requires nova:sessions:admin")
		return
	}
	uid := strings.TrimSpace(chi.URLParam(r, "id"))
	if uid == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", "user id is required")
		return
	}
	q := url.Values{}
	for k, vs := range r.URL.Query() {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	q.Set("user", uid)
	if q.Get("type") == "" {
		q.Set("type", "LOGIN")
	}
	resp, err := h.Admin.Do(r.Context(), http.MethodGet, "/events", q, nil)
	h.proxy(w, resp, err, "user.login_history")
}
