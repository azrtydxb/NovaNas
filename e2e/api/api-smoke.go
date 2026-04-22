// Package apismoke exercises the NovaNas HTTP API's 14 route modules at a
// smoke-test level. It is consumed by api-smoke_test.go but also runnable as
// a standalone binary for ad-hoc CI probing.
package apismoke

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Endpoint describes a route under test. Status in StatusCodes is accepted.
type Endpoint struct {
	Method      string
	Path        string
	StatusCodes []int
	RequiresAuth bool
	Note        string
}

// Routes lists the minimum happy-path probes across NovaNas's 14 API modules.
// Non-2xx/4xx semantics are deliberately permissive: we assert only that the
// route exists and is authenticated, not that a fixture is present.
var Routes = []Endpoint{
	{Method: "GET", Path: "/health", StatusCodes: []int{200}, Note: "liveness"},
	{Method: "GET", Path: "/ready", StatusCodes: []int{200, 503}, Note: "readiness"},
	{Method: "GET", Path: "/api/version", StatusCodes: []int{200}, Note: "version"},

	// 14 route modules — list endpoints only, list is OK empty.
	{Method: "GET", Path: "/api/v1/pools", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/datasets", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/buckets", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/shares", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/disks", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/snapshots", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/snapshots/schedules", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/replication/targets", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/replication/jobs", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/apps/catalog", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/apps/instances", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/vms", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/system/settings", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/system/alerts", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/system/audit", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/system/updates", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/identity/users", StatusCodes: []int{200}, RequiresAuth: true},
	{Method: "GET", Path: "/api/v1/identity/groups", StatusCodes: []int{200}, RequiresAuth: true},
}

// Config controls where the smokes run.
type Config struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

// ConfigFromEnv loads E2E_BASE_URL / E2E_API_TOKEN with sensible defaults.
func ConfigFromEnv() Config {
	cfg := Config{
		BaseURL: getenv("NOVANAS_BASE_URL", "https://localhost:8443"),
		Token:   os.Getenv("E2E_API_TOKEN"),
		Timeout: 10 * time.Second,
	}
	return cfg
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Result is returned per endpoint.
type Result struct {
	Endpoint Endpoint
	Status   int
	Err      error
}

// Ok reports whether the response status matches any allowed code.
func (r Result) Ok() bool {
	if r.Err != nil {
		return false
	}
	for _, s := range r.Endpoint.StatusCodes {
		if r.Status == s {
			return true
		}
	}
	return false
}

// Run probes every endpoint in Routes and returns a slice of results.
func Run(ctx context.Context, cfg Config) []Result {
	client := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // E2E self-signed
		},
	}
	results := make([]Result, 0, len(Routes))
	for _, ep := range Routes {
		req, err := http.NewRequestWithContext(ctx, ep.Method, cfg.BaseURL+ep.Path, nil)
		if err != nil {
			results = append(results, Result{Endpoint: ep, Err: err})
			continue
		}
		if ep.RequiresAuth && cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
		}
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			results = append(results, Result{Endpoint: ep, Err: err})
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		results = append(results, Result{Endpoint: ep, Status: resp.StatusCode})
	}
	return results
}

// Main lets this package function as a standalone binary via a thin wrapper
// (cmd/smoke/main.go if added later); here we provide a helper for tests and
// scripts that prefers JSON output when NOVANAS_JSON=1 is set.
func PrintResults(results []Result) int {
	failures := 0
	if os.Getenv("NOVANAS_JSON") == "1" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
		for _, r := range results {
			if !r.Ok() {
				failures++
			}
		}
		return failures
	}
	for _, r := range results {
		mark := "PASS"
		if !r.Ok() {
			mark = "FAIL"
			failures++
		}
		msg := fmt.Sprintf("%s  %-6s %-40s  → %d", mark, r.Endpoint.Method, r.Endpoint.Path, r.Status)
		if r.Err != nil {
			msg += "  err=" + r.Err.Error()
		}
		fmt.Println(msg)
	}
	return failures
}
