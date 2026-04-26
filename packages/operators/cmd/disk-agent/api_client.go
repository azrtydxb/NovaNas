package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// apiClient is a thin HTTP client for the NovaNas API. The disk-agent
// uses it to upsert Disk records; it replaces the previous direct
// kube-apiserver path (dynamic.Resource(diskGVR).Create / Patch).
//
// Authentication: the agent presents its pod's projected ServiceAccount
// JWT in `Authorization: Bearer …`. The API verifies it via Kubernetes
// TokenReview (see packages/api/src/auth/tokenreview.ts) and maps it
// to the `internal:disk-agent` principal.
//
// TLS: the agent trusts the in-cluster CA bundle (the same one
// kubelet projects into every pod) so it can validate the API's
// cert-manager-issued cert.
type apiClient struct {
	base       string
	token      string
	httpClient *http.Client
}

// envOrDefault reads an env var, returning a default when unset.
func envOrDefault(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// newApiClient constructs a client. Env vars consumed:
//
//	NOVANAS_API_URL  (default: https://novanas-api.novanas-system.svc)
//	NOVANAS_API_TOKEN_PATH (default: /var/run/secrets/kubernetes.io/serviceaccount/token)
//	NOVANAS_API_CA_PATH (default: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt)
//	NOVANAS_API_INSECURE (default: ""; set "1" to skip TLS verification — dev only)
func newApiClient() (*apiClient, error) {
	base := strings.TrimRight(
		envOrDefault("NOVANAS_API_URL", "https://novanas-api.novanas-system.svc"),
		"/",
	)
	tokenPath := envOrDefault(
		"NOVANAS_API_TOKEN_PATH",
		"/var/run/secrets/kubernetes.io/serviceaccount/token",
	)
	tokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("read sa token from %s: %w", tokenPath, err)
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return nil, fmt.Errorf("sa token at %s is empty", tokenPath)
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if os.Getenv("NOVANAS_API_INSECURE") == "1" {
		tlsCfg.InsecureSkipVerify = true
	} else {
		caPath := envOrDefault(
			"NOVANAS_API_CA_PATH",
			"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		)
		if data, err := os.ReadFile(caPath); err == nil {
			pool, _ := x509.SystemCertPool()
			if pool == nil {
				pool = x509.NewCertPool()
			}
			pool.AppendCertsFromPEM(data)
			tlsCfg.RootCAs = pool
		}
		// If the CA is unreachable we fall back to the system pool —
		// the API may be served by a publicly trusted cert in some
		// deployments, and TLS verification should still happen.
	}

	return &apiClient{
		base:  base,
		token: token,
		httpClient: &http.Client{
			Timeout:   15 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}, nil
}

// disk represents the NovaNas API's wire shape for the Disk resource.
// Mirrors `@novanas/schemas` DiskSchema. We marshal/unmarshal via
// generic map[string]any so the agent doesn't pin a dep on a typed
// schema package.
type diskEnvelope struct {
	APIVersion string                 `json:"apiVersion"`
	Kind       string                 `json:"kind"`
	Metadata   diskMeta               `json:"metadata"`
	Spec       map[string]any         `json:"spec"`
	Status     map[string]any         `json:"status,omitempty"`
}
type diskMeta struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

func (c *apiClient) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *apiClient) do(req *http.Request, out any) error {
	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return apiError{Status: res.StatusCode, Body: string(body)}
	}
	if out != nil && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("unmarshal: %w (body=%s)", err, string(body))
		}
	}
	return nil
}

type apiError struct {
	Status int
	Body   string
}

func (e apiError) Error() string {
	return fmt.Sprintf("api %d: %s", e.Status, e.Body)
}

// IsNotFound reports whether err represents an HTTP 404 from the API.
func IsNotFound(err error) bool {
	if e, ok := err.(apiError); ok {
		return e.Status == 404
	}
	return false
}

// GetDisk returns the disk envelope by name, or nil if it doesn't exist.
func (c *apiClient) GetDisk(ctx context.Context, name string) (*diskEnvelope, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/disks/"+name, nil)
	if err != nil {
		return nil, err
	}
	out := &diskEnvelope{}
	if err := c.do(req, out); err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return out, nil
}

// CreateDisk POSTs a new disk record. The route's hydration logic on
// the API side adds apiVersion/kind, but we send them anyway for
// readability.
func (c *apiClient) CreateDisk(ctx context.Context, body *diskEnvelope) (*diskEnvelope, error) {
	body.APIVersion = "novanas.io/v1alpha1"
	body.Kind = "Disk"
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/disks", body)
	if err != nil {
		return nil, err
	}
	out := &diskEnvelope{}
	if err := c.do(req, out); err != nil {
		return nil, err
	}
	return out, nil
}

// PatchDisk applies a JSON-merge patch to an existing disk. The API
// uses MergePatch semantics: top-level keys (metadata.labels,
// metadata.annotations, spec, status) are replaced; nested keys merge.
func (c *apiClient) PatchDisk(ctx context.Context, name string, patch map[string]any) (*diskEnvelope, error) {
	req, err := c.newRequest(ctx, http.MethodPatch, "/api/v1/disks/"+name, patch)
	if err != nil {
		return nil, err
	}
	out := &diskEnvelope{}
	if err := c.do(req, out); err != nil {
		return nil, err
	}
	return out, nil
}
