// Package novanas is a Go SDK for the NovaNAS HTTP API.
//
// This module is intentionally a separate Go module so external consumers
// (e.g. the NovaNAS CSI driver) can `go get` it without pulling in any of
// the NovaNAS server's internal packages. It depends only on the standard
// library.
package novanas

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
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

// DefaultTimeout is used by New when Config.Timeout is zero.
const DefaultTimeout = 30 * time.Second

// DefaultUserAgent is the User-Agent header sent on every request unless the
// caller overrides Client.UserAgent.
const DefaultUserAgent = "novanas-go/1"

// apiPrefix is the path prefix every endpoint lives under on the server.
const apiPrefix = "/api/v1"

// Client is an HTTP client for the NovaNAS API. Construct one with New, or
// build it directly if you need full control over the embedded http.Client
// (e.g. to share a transport with metrics middleware).
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	// Token is the bearer token used on every request. Direct field access
	// remains supported for back-compat (tests and callers that constructed
	// Client by hand). For concurrent rotation (e.g. an OIDC refresh
	// goroutine) callers MUST use SetToken / token() instead of touching
	// Token directly, since those paths take the mutex.
	Token     string
	UserAgent string

	// tokenMu guards Token when SetToken is in use. The zero value is fine;
	// callers that never call SetToken pay only an uncontended RLock per
	// request.
	tokenMu sync.RWMutex
}

// SetToken atomically replaces the bearer token used by subsequent requests.
// It is safe to call from a background goroutine (e.g. a token-refresh loop)
// while requests are in flight. Existing in-flight requests are unaffected;
// the new token applies to requests started after SetToken returns.
func (c *Client) SetToken(token string) {
	c.tokenMu.Lock()
	c.Token = token
	c.tokenMu.Unlock()
}

// token returns the current bearer token under the read lock.
func (c *Client) token() string {
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.Token
}

// Config controls how New constructs a Client. All fields are optional except
// BaseURL.
type Config struct {
	BaseURL            string
	Token              string
	InsecureSkipVerify bool
	CACertPEM          []byte
	Timeout            time.Duration
}

// New constructs a Client from cfg. It builds a *http.Client with the TLS
// configuration implied by InsecureSkipVerify and CACertPEM.
//
// Both, neither, or one of (CACertPEM, InsecureSkipVerify) is valid. When
// both are set, the CA pool is built but verification is still skipped —
// useful in dev when you want the pool wired up but don't want to fight
// hostname mismatches.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("novanas: BaseURL is required")
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	tlsCfg := &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify} //nolint:gosec // operator opt-in
	if len(cfg.CACertPEM) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.CACertPEM) {
			return nil, errors.New("novanas: CACertPEM contains no valid certificates")
		}
		tlsCfg.RootCAs = pool
	}

	transport := &http.Transport{
		TLSClientConfig:       tlsCfg,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: timeout,
	}
	return &Client{
		BaseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		HTTPClient: &http.Client{Transport: transport, Timeout: timeout},
		Token:      cfg.Token,
		UserAgent:  DefaultUserAgent,
	}, nil
}

// do performs an HTTP request. If body is non-nil it is JSON-encoded and
// Content-Type: application/json is set. If out is non-nil and the response
// status is in [200, 300), the body is JSON-decoded into out.
//
// On a non-2xx response, do returns an *APIError populated from the standard
// {"error": "...", "message": "..."} envelope. Network errors are returned
// as-is.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) (*http.Response, error) {
	u := c.BaseURL + apiPrefix + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("novanas: marshal request body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if ua := c.UserAgent; ua != "" {
		req.Header.Set("User-Agent", ua)
	} else {
		req.Header.Set("User-Agent", DefaultUserAgent)
	}
	if tok := c.token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		buf, _ := io.ReadAll(resp.Body)
		var env struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(buf, &env)
		return resp, &APIError{
			StatusCode: resp.StatusCode,
			Code:       env.Error,
			Message:    env.Message,
		}
	}

	if out != nil {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp, fmt.Errorf("novanas: decode response: %w", err)
		}
	}
	return resp, nil
}

// jobIDFromLocation extracts the trailing UUID segment from a Location
// header value of the form "/api/v1/jobs/<uuid>". Returns "" if the
// header is missing or malformed.
func jobIDFromLocation(loc string) string {
	if loc == "" {
		return ""
	}
	// Strip optional scheme/host.
	if i := strings.Index(loc, "/jobs/"); i >= 0 {
		return strings.TrimSpace(loc[i+len("/jobs/"):])
	}
	return ""
}

// finishJobFromAccepted reads the 202 envelope (currently
// {"jobId":"<uuid>"}) and returns a Job stub. The server does not include
// the full Job representation in the 202 body — only the ID — so this
// stub is intentionally minimal. Callers that want the full record should
// follow up with GetJob or WaitJob.
func finishJobFromAccepted(resp *http.Response) (*Job, error) {
	defer resp.Body.Close()
	var env struct {
		JobID string `json:"jobId"`
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) > 0 {
		_ = json.Unmarshal(body, &env)
	}
	id := strings.TrimSpace(env.JobID)
	if id == "" {
		id = jobIDFromLocation(resp.Header.Get("Location"))
	}
	if id == "" {
		return nil, fmt.Errorf("novanas: 202 response missing job id (status=%d)", resp.StatusCode)
	}
	return &Job{ID: id, State: JobStateQueued}, nil
}

// boolQuery is a tiny helper to keep call sites tidy.
func boolQuery(v bool) string { return strconv.FormatBool(v) }
