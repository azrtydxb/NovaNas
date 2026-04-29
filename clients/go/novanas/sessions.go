package novanas

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

// Session mirrors a Keycloak user-session entry as exposed by
// /api/v1/auth/sessions. Fields follow Keycloak admin's UserSessionRepresentation.
type Session struct {
	ID         string            `json:"id"`
	Username   string            `json:"username,omitempty"`
	UserID     string            `json:"userId,omitempty"`
	IPAddress  string            `json:"ipAddress,omitempty"`
	Start      int64             `json:"start,omitempty"`
	LastAccess int64             `json:"lastAccess,omitempty"`
	Clients    map[string]string `json:"clients,omitempty"`
	// Type distinguishes user-sessions from offline-sessions on the
	// server side. Empty when the upstream doesn't supply it.
	Type string `json:"type,omitempty"`
}

// LoginEvent mirrors Keycloak's EventRepresentation, filtered to LOGIN.
type LoginEvent struct {
	Time      int64             `json:"time"`
	Type      string            `json:"type"`
	RealmID   string            `json:"realmId,omitempty"`
	ClientID  string            `json:"clientId,omitempty"`
	UserID    string            `json:"userId,omitempty"`
	IPAddress string            `json:"ipAddress,omitempty"`
	Error     string            `json:"error,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

// ListOwnSessions returns the caller's active Keycloak sessions.
func (c *Client) ListOwnSessions(ctx context.Context) ([]Session, error) {
	var out []Session
	if _, err := c.do(ctx, http.MethodGet, "/auth/sessions", nil, nil, &out); err != nil {
		// Keycloak admin may return raw arrays or wrap them; on decode
		// failure surface the error.
		return nil, err
	}
	return out, nil
}

// RevokeOwnSession revokes one of the caller's sessions.
func (c *Client) RevokeOwnSession(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("novanas: session id is required")
	}
	_, err := c.do(ctx, http.MethodDelete, "/auth/sessions/"+url.PathEscape(id), nil, nil, nil)
	return err
}

// ListUserSessions returns sessions for an arbitrary user (admin-only).
func (c *Client) ListUserSessions(ctx context.Context, userID string) ([]Session, error) {
	if userID == "" {
		return nil, errors.New("novanas: userID is required")
	}
	var out []Session
	if _, err := c.do(ctx, http.MethodGet, "/auth/users/"+url.PathEscape(userID)+"/sessions", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RevokeAllUserSessions logs the user out of every active Keycloak
// session (admin-only).
func (c *Client) RevokeAllUserSessions(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("novanas: userID is required")
	}
	_, err := c.do(ctx, http.MethodDelete, "/auth/users/"+url.PathEscape(userID)+"/sessions", nil, nil, nil)
	return err
}

// ListOwnLoginHistory returns the caller's recent LOGIN events. extra
// is forwarded to Keycloak's /events query (e.g. dateFrom, dateTo, max,
// first).
func (c *Client) ListOwnLoginHistory(ctx context.Context, extra url.Values) ([]LoginEvent, error) {
	var out []LoginEvent
	if _, err := c.do(ctx, http.MethodGet, "/auth/login-history", extra, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListUserLoginHistory returns LOGIN events for an arbitrary user
// (admin-only).
func (c *Client) ListUserLoginHistory(ctx context.Context, userID string, extra url.Values) ([]LoginEvent, error) {
	if userID == "" {
		return nil, errors.New("novanas: userID is required")
	}
	var out []LoginEvent
	if _, err := c.do(ctx, http.MethodGet, "/auth/users/"+url.PathEscape(userID)+"/login-history", extra, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
