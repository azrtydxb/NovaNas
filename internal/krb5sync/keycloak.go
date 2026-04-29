// Package krb5sync reconciles Keycloak users with the embedded MIT KDC's
// principal database. See cmd/nova-krb5-sync for the daemon entrypoint and
// docs/krb5/sync.md for the operator-facing documentation.
package krb5sync

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// KeycloakUser is the slice of the Keycloak admin user representation we
// care about. Keycloak returns many more fields; we deliberately don't
// model them to keep the surface small.
type KeycloakUser struct {
	ID         string              `json:"id"`
	Username   string              `json:"username"`
	Enabled    bool                `json:"enabled"`
	Attributes map[string][]string `json:"attributes,omitempty"`
}

// Tenants returns the values of the `nova-tenant` attribute. An empty
// slice means the user is platform-level (admin/ops).
func (u KeycloakUser) Tenants() []string {
	return u.Attributes[TenantAttribute]
}

// PlatformNFSEnabled reports whether the user has `nova-platform-nfs: true`.
// Platform-level users (no tenant attribute) only get a bare-username
// principal when this flag is true.
func (u KeycloakUser) PlatformNFSEnabled() bool {
	for _, v := range u.Attributes[PlatformNFSAttribute] {
		if strings.EqualFold(strings.TrimSpace(v), "true") {
			return true
		}
	}
	return false
}

// Constants for the attribute model. These names are part of the operator
// contract (see docs/krb5/sync.md) and changing them is a breaking change.
const (
	// TenantAttribute is the Keycloak user attribute carrying tenant IDs.
	TenantAttribute = "nova-tenant"
	// PlatformNFSAttribute opts platform-level users into a bare-username
	// principal for NFS access.
	PlatformNFSAttribute = "nova-platform-nfs"
)

// AdminEvent is the slice of the Keycloak admin-event representation we
// react to. The relevant operations are CREATE/UPDATE/DELETE on USER
// resources.
type AdminEvent struct {
	Time         int64  `json:"time"`
	RealmID      string `json:"realmId"`
	OperationType string `json:"operationType"`
	ResourceType string `json:"resourceType"`
	ResourcePath string `json:"resourcePath"`
}

// KeycloakAPI is the subset of the Keycloak admin API we depend on. The
// interface exists so sync_test.go can drop in a fake without spinning up
// a real Keycloak instance.
type KeycloakAPI interface {
	ListUsers(ctx context.Context) ([]KeycloakUser, error)
	GetUser(ctx context.Context, id string) (*KeycloakUser, error)
	ListAdminEvents(ctx context.Context, since time.Time) ([]AdminEvent, error)
}

// KeycloakClient talks to the Keycloak admin API using a Bearer token
// obtained via the OAuth2 client_credentials grant. It is safe for
// concurrent use; the token is rotated by a background refresh goroutine.
type KeycloakClient struct {
	BaseURL string // e.g. https://kc:8443
	Realm   string
	HTTP    *http.Client

	tokenSrc *kcTokenSource
}

// KeycloakConfig configures NewKeycloakClient.
type KeycloakConfig struct {
	BaseURL            string
	Realm              string
	ClientID           string
	ClientSecret       string
	CACertPEM          []byte
	InsecureSkipVerify bool
	Timeout            time.Duration
}

// NewKeycloakClient builds a Keycloak admin API client and performs the
// initial token fetch. It fails closed on auth errors so the caller can
// exit cleanly.
func NewKeycloakClient(ctx context.Context, cfg KeycloakConfig) (*KeycloakClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("krb5sync: keycloak BaseURL is required")
	}
	if strings.TrimSpace(cfg.Realm) == "" {
		return nil, errors.New("krb5sync: keycloak Realm is required")
	}
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, errors.New("krb5sync: keycloak ClientID and ClientSecret are required")
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.InsecureSkipVerify} //nolint:gosec // operator opt-in
	if len(cfg.CACertPEM) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.CACertPEM) {
			return nil, errors.New("krb5sync: keycloak CACertPEM contained no valid certificates")
		}
		tlsCfg.RootCAs = pool
	}
	httpc := &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{TLSClientConfig: tlsCfg, ForceAttemptHTTP2: true},
	}
	src := &kcTokenSource{
		tokenURL:     strings.TrimRight(cfg.BaseURL, "/") + "/realms/" + url.PathEscape(cfg.Realm) + "/protocol/openid-connect/token",
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		httpc:        httpc,
	}
	if _, _, err := src.fetch(ctx); err != nil {
		return nil, fmt.Errorf("krb5sync: initial keycloak token fetch: %w", err)
	}
	return &KeycloakClient{
		BaseURL:  strings.TrimRight(cfg.BaseURL, "/"),
		Realm:    cfg.Realm,
		HTTP:     httpc,
		tokenSrc: src,
	}, nil
}

// authorizedRequest builds an HTTP request with a fresh bearer token.
func (c *KeycloakClient) authorizedRequest(ctx context.Context, method, path string, query url.Values) (*http.Request, error) {
	tok, err := c.tokenSrc.Token(ctx)
	if err != nil {
		return nil, err
	}
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// ListUsers fetches all users in the realm via paginated GETs against
// /admin/realms/<realm>/users. Paginates with `first` + `max=100`.
func (c *KeycloakClient) ListUsers(ctx context.Context) ([]KeycloakUser, error) {
	const pageSize = 100
	out := make([]KeycloakUser, 0, pageSize)
	first := 0
	for {
		q := url.Values{}
		q.Set("first", strconv.Itoa(first))
		q.Set("max", strconv.Itoa(pageSize))
		q.Set("briefRepresentation", "false")
		req, err := c.authorizedRequest(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(c.Realm)+"/users", q)
		if err != nil {
			return nil, err
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("krb5sync: list users: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("krb5sync: list users: HTTP %d: %s", resp.StatusCode, truncate(string(body), 256))
		}
		var page []KeycloakUser
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("krb5sync: decode users page: %w", err)
		}
		out = append(out, page...)
		if len(page) < pageSize {
			break
		}
		first += len(page)
	}
	return out, nil
}

// GetUser fetches one user by ID. Returns nil with no error if the user
// is missing (404).
func (c *KeycloakClient) GetUser(ctx context.Context, id string) (*KeycloakUser, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("krb5sync: user id is required")
	}
	req, err := c.authorizedRequest(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(c.Realm)+"/users/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("krb5sync: get user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("krb5sync: get user: HTTP %d: %s", resp.StatusCode, truncate(string(body), 256))
	}
	var u KeycloakUser
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("krb5sync: decode user: %w", err)
	}
	return &u, nil
}

// ListAdminEvents fetches admin events since the given time. Empty `since`
// means "no lower bound" (Keycloak returns all retained events).
func (c *KeycloakClient) ListAdminEvents(ctx context.Context, since time.Time) ([]AdminEvent, error) {
	q := url.Values{}
	q.Set("max", "200")
	if !since.IsZero() {
		q.Set("dateFrom", since.UTC().Format("2006-01-02"))
	}
	// Filter to USER resource events only.
	q.Set("resourceTypes", "USER")
	req, err := c.authorizedRequest(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(c.Realm)+"/admin-events", q)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("krb5sync: list admin events: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("krb5sync: list admin events: HTTP %d: %s", resp.StatusCode, truncate(string(body), 256))
	}
	var events []AdminEvent
	if err := json.Unmarshal(body, &events); err != nil {
		return nil, fmt.Errorf("krb5sync: decode admin events: %w", err)
	}
	return events, nil
}

// -----------------------------------------------------------------------
// kcTokenSource: client_credentials with cached access token
// -----------------------------------------------------------------------

type kcTokenSource struct {
	tokenURL     string
	clientID     string
	clientSecret string
	httpc        *http.Client

	mu  sync.Mutex
	tok string
	exp time.Time
}

// Token returns a non-expired bearer token, refreshing if needed.
func (s *kcTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.tok != "" && time.Until(s.exp) > 30*time.Second {
		t := s.tok
		s.mu.Unlock()
		return t, nil
	}
	s.mu.Unlock()
	tok, exp, err := s.fetch(ctx)
	if err != nil {
		return "", err
	}
	_ = exp
	return tok, nil
}

func (s *kcTokenSource) fetch(ctx context.Context) (string, time.Time, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", s.clientID)
	form.Set("client_secret", s.clientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpc.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("krb5sync: keycloak token endpoint: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, fmt.Errorf("krb5sync: keycloak token endpoint returned %d: %s", resp.StatusCode, truncate(string(body), 256))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", time.Time{}, fmt.Errorf("krb5sync: decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", time.Time{}, errors.New("krb5sync: empty access_token")
	}
	exp := time.Time{}
	if jwtExp, ok := decodeJWTExp(tr.AccessToken); ok {
		exp = jwtExp
	} else if tr.ExpiresIn > 0 {
		exp = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	} else {
		exp = time.Now().Add(60 * time.Second)
	}
	s.mu.Lock()
	s.tok = tr.AccessToken
	s.exp = exp
	s.mu.Unlock()
	return tr.AccessToken, exp, nil
}

func decodeJWTExp(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return time.Time{}, false
		}
	}
	var claims struct {
		Exp json.Number `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, false
	}
	expSec, err := claims.Exp.Int64()
	if err != nil || expSec <= 0 {
		return time.Time{}, false
	}
	return time.Unix(expSec, 0), true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
