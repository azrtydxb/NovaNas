// Package reconciler — HTTP-backed OpenBaoClient.
//
// HTTPOpenBaoClient implements the OpenBaoClient interface by talking
// directly to the OpenBao REST API. It is the production counterpart
// to NoopOpenBaoClient/FakeOpenBaoClient.
//
// Endpoints used (same shape as HashiCorp Vault):
//
//	PUT    /v1/sys/policies/acl/<name>        write policy
//	DELETE /v1/sys/policies/acl/<name>        delete policy
//	POST   /v1/auth/kubernetes/role/<name>    create / update role
//	DELETE /v1/auth/kubernetes/role/<name>    delete role
//
// The token is either taken from the static Token field or re-read
// from TokenPath on every call (supports projected SA tokens that
// rotate on disk).
package reconciler

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// HTTPOpenBaoConfig configures an HTTPOpenBaoClient.
type HTTPOpenBaoConfig struct {
	// Addr is the OpenBao base URL (e.g. "https://openbao:8200").
	Addr string
	// Token is a static token; if empty TokenPath is read on each call.
	Token string
	// TokenPath is the file holding the (possibly rotating) token.
	TokenPath string
	// Namespace is the enterprise X-Vault-Namespace header.
	Namespace string
	// KubernetesAuthMount is the mount path for the Kubernetes auth
	// method (default "kubernetes"). The full role URL is
	// /v1/auth/<mount>/role/<name>.
	KubernetesAuthMount string
	// InsecureSkipVerify disables TLS verification. DO NOT set outside
	// dev — defaults to false (verify on).
	InsecureSkipVerify bool
	// Timeout per HTTP request.
	Timeout time.Duration
}

// HTTPOpenBaoClient is the production OpenBaoClient. Safe for
// concurrent use.
type HTTPOpenBaoClient struct {
	cfg    HTTPOpenBaoConfig
	client *http.Client
}

// NewHTTPOpenBaoClient validates the config and returns a new client.
// baseURL is required; at least one of token or OPENBAO_TOKEN_PATH env
// (or cfg.TokenPath) must be set.
func NewHTTPOpenBaoClient(baseURL, token string) (*HTTPOpenBaoClient, error) {
	cfg := HTTPOpenBaoConfig{
		Addr:                baseURL,
		Token:               token,
		TokenPath:           os.Getenv("OPENBAO_TOKEN_PATH"),
		Namespace:           os.Getenv("OPENBAO_NAMESPACE"),
		KubernetesAuthMount: "kubernetes",
		Timeout:             15 * time.Second,
	}
	return NewHTTPOpenBaoClientWithConfig(cfg)
}

// NewHTTPOpenBaoClientWithConfig is the full-config constructor.
func NewHTTPOpenBaoClientWithConfig(cfg HTTPOpenBaoConfig) (*HTTPOpenBaoClient, error) {
	if strings.TrimSpace(cfg.Addr) == "" {
		return nil, errors.New("openbao: Addr is required")
	}
	if cfg.Token == "" && cfg.TokenPath == "" {
		return nil, errors.New("openbao: Token or TokenPath is required")
	}
	if cfg.KubernetesAuthMount == "" {
		cfg.KubernetesAuthMount = "kubernetes"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // operator-controlled
	}
	return &HTTPOpenBaoClient{
		cfg: cfg,
		client: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}, nil
}

// EnsurePolicy writes (create-or-update) an ACL policy.
func (c *HTTPOpenBaoClient) EnsurePolicy(ctx context.Context, p OpenBaoPolicy) error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("openbao: policy name is required")
	}
	path := fmt.Sprintf("sys/policies/acl/%s", p.Name)
	body := map[string]any{"policy": p.HCL}
	if _, err := c.do(ctx, http.MethodPut, path, body); err != nil {
		return fmt.Errorf("openbao: write policy %q: %w", p.Name, err)
	}
	return nil
}

// DeletePolicy removes an ACL policy. Missing policies are treated as
// success (404 swallowed).
func (c *HTTPOpenBaoClient) DeletePolicy(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("openbao: policy name is required")
	}
	path := fmt.Sprintf("sys/policies/acl/%s", name)
	if _, err := c.do(ctx, http.MethodDelete, path, nil); err != nil {
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("openbao: delete policy %q: %w", name, err)
	}
	return nil
}

// EnsureAuthRole creates or updates a Kubernetes-auth role.
func (c *HTTPOpenBaoClient) EnsureAuthRole(ctx context.Context, r OpenBaoAuthRole) error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("openbao: role name is required")
	}
	body := map[string]any{
		"bound_service_account_names":      splitOrSelf(r.BoundServiceAccount),
		"bound_service_account_namespaces": splitOrSelf(r.BoundNamespace),
		"policies":                         r.Policies,
	}
	if r.TTLSeconds > 0 {
		body["token_ttl"] = fmt.Sprintf("%ds", r.TTLSeconds)
	}
	if r.MaxTTLSeconds > 0 {
		body["token_max_ttl"] = fmt.Sprintf("%ds", r.MaxTTLSeconds)
	}
	path := fmt.Sprintf("auth/%s/role/%s", c.cfg.KubernetesAuthMount, r.Name)
	if _, err := c.do(ctx, http.MethodPost, path, body); err != nil {
		return fmt.Errorf("openbao: write auth role %q: %w", r.Name, err)
	}
	return nil
}

// DeleteAuthRole removes a Kubernetes-auth role. Missing roles are
// treated as success.
func (c *HTTPOpenBaoClient) DeleteAuthRole(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("openbao: role name is required")
	}
	path := fmt.Sprintf("auth/%s/role/%s", c.cfg.KubernetesAuthMount, name)
	if _, err := c.do(ctx, http.MethodDelete, path, nil); err != nil {
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("openbao: delete auth role %q: %w", name, err)
	}
	return nil
}

// splitOrSelf returns the single-element slice [s] if s is non-empty,
// else a nil slice. A single SA/namespace is the common case.
func splitOrSelf(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return []string{s}
}

func (c *HTTPOpenBaoClient) token() (string, error) {
	if c.cfg.TokenPath != "" {
		data, err := os.ReadFile(c.cfg.TokenPath)
		if err != nil {
			// fall through to static token if any
			if c.cfg.Token != "" {
				return c.cfg.Token, nil
			}
			return "", fmt.Errorf("read token file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return c.cfg.Token, nil
}

func (c *HTTPOpenBaoClient) do(ctx context.Context, method, path string, body any) (map[string]any, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(buf)
	}
	url := strings.TrimRight(c.cfg.Addr, "/") + "/v1/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, err
	}
	tok, err := c.token()
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", tok)
	if c.cfg.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.cfg.Namespace)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		if resp.StatusCode == http.StatusNotFound {
			return nil, &httpStatusError{Status: resp.StatusCode, Body: string(buf)}
		}
		return nil, fmt.Errorf("%s %s: %d: %s", method, path, resp.StatusCode, string(buf))
	}
	if len(buf) == 0 {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return out, nil
}

// httpStatusError carries an HTTP status so callers can special-case
// 404 without string matching.
type httpStatusError struct {
	Status int
	Body   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("http %d: %s", e.Status, e.Body)
}

func isNotFound(err error) bool {
	var se *httpStatusError
	if errors.As(err, &se) {
		return se.Status == http.StatusNotFound
	}
	return false
}

var _ OpenBaoClient = (*HTTPOpenBaoClient)(nil)
