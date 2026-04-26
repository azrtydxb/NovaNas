// Package novanas is the Go client SDK for the NovaNas API.
//
// Built for in-cluster consumers (storage controllers, host agents)
// that previously watched CRDs via controller-runtime. Each consumer
// presents its projected ServiceAccount JWT as the bearer; api-side
// TokenReview validates and maps the SA onto an internal: role.
//
// The surface is deliberately minimal — list + get + status patch on
// the resource kinds the data plane needs. Resource type definitions
// mirror the Postgres-backed schema in @novanas/schemas (TypeScript)
// but only carry the fields the storage data plane actually reads.
package novanas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client wraps an authenticated HTTP session against the NovaNas API.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewFromEnv builds a Client from NOVANAS_API_URL and a projected SA
// token at NOVANAS_API_TOKEN_FILE (defaults to /var/run/secrets/
// kubernetes.io/serviceaccount/token).
//
// Both env vars are populated by the helm chart's pod spec.
func NewFromEnv() (*Client, error) {
	base := os.Getenv("NOVANAS_API_URL")
	if base == "" {
		return nil, fmt.Errorf("NOVANAS_API_URL is unset")
	}
	tokenPath := os.Getenv("NOVANAS_API_TOKEN_FILE")
	if tokenPath == "" {
		tokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	}
	tok, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("read token from %s: %w", tokenPath, err)
	}
	return New(base, string(bytes.TrimSpace(tok))), nil
}

// New constructs a Client with explicit baseURL + bearer token.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// listResult is the wire shape for `GET /api/v1/<plural>` responses.
type listResult[T any] struct {
	Items []T `json:"items"`
}

// APIError is returned when the api responds with a non-2xx status.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("novanas-api %d: %s", e.Status, e.Body)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Body: string(respBody)}
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

// list issues a GET /api/v1/<plural> and parses the standard
// {items: T[]} envelope.
func list[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var lr listResult[T]
	if err := c.do(ctx, http.MethodGet, path, nil, &lr); err != nil {
		return nil, err
	}
	return lr.Items, nil
}

// patchStatus issues PATCH /api/v1/<plural>/<name> with a status-only
// merge-patch body. The api routes status writes through the same
// PgResource.patch path used for spec/labels/annotations updates.
func (c *Client) patchStatus(ctx context.Context, path string, status any) error {
	return c.do(ctx, http.MethodPatch, path, map[string]any{"status": status}, nil)
}
