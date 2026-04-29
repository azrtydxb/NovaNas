package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestHTTPMiddleware_CountsAndDuration drives a chi router with the
// middleware installed and asserts both the request counter and the
// duration histogram pick up labelled samples.
func TestHTTPMiddleware_CountsAndDuration(t *testing.T) {
	m := New()

	r := chi.NewRouter()
	r.Use(m.HTTP.Middleware)
	r.Get("/api/v1/pools/{name}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	r.Post("/api/v1/pools", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	r.Get("/metrics", m.Handler().ServeHTTP)

	cases := []struct {
		method, path string
		wantStatus   int
	}{
		{"GET", "/api/v1/pools/tank", 200},
		{"GET", "/api/v1/pools/data", 200}, // same chi pattern, counter shares label set
		{"POST", "/api/v1/pools", 400},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != c.wantStatus {
			t.Fatalf("%s %s: status=%d want %d", c.method, c.path, rr.Code, c.wantStatus)
		}
	}

	body := scrape(t, r)

	wantSubstrings := []string{
		// Two requests collapsed onto the same chi-matched pattern.
		`nova_http_requests_total{method="GET",path="/api/v1/pools/{name}",status="200"} 2`,
		`nova_http_requests_total{method="POST",path="/api/v1/pools",status="400"} 1`,
		// Histogram count line; existence is enough — bucket boundaries
		// are an implementation detail of the bucket spec.
		`nova_http_request_duration_seconds_count{method="GET",path="/api/v1/pools/{name}"} 2`,
	}
	for _, s := range wantSubstrings {
		if !strings.Contains(body, s) {
			t.Errorf("scrape body missing %q\n--- body ---\n%s", s, body)
		}
	}
}

// TestHTTPMiddleware_SkipsMetricsPath verifies that scraping /metrics
// itself does not inflate the request counter — otherwise every scrape
// would create unbounded growth in the time series.
func TestHTTPMiddleware_SkipsMetricsPath(t *testing.T) {
	m := New()
	r := chi.NewRouter()
	r.Use(m.HTTP.Middleware)
	r.Get("/metrics", m.Handler().ServeHTTP)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/metrics", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
	}

	body := scrape(t, r)
	if strings.Contains(body, `nova_http_requests_total{method="GET",path="/metrics"`) {
		t.Errorf("middleware should skip /metrics, but counter recorded it:\n%s", body)
	}
}

// scrape calls /metrics on the given router and returns the body.
func scrape(t *testing.T, h http.Handler) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("scrape /metrics: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("scrape Content-Type=%q want text/plain*", ct)
	}
	return rr.Body.String()
}
