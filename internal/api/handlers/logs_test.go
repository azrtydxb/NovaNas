package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newLogsTestServer(t *testing.T, upstream http.Handler) *chi.Mux {
	t.Helper()
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	h := &LogsHandler{UpstreamURL: up.URL, HTTP: up.Client()}
	r := chi.NewRouter()
	r.Get("/api/v1/logs/query", h.QueryRange)
	r.Get("/api/v1/logs/query/instant", h.QueryInstant)
	r.Get("/api/v1/logs/labels", h.Labels)
	r.Get("/api/v1/logs/labels/{name}/values", h.LabelValues)
	r.Get("/api/v1/logs/series", h.Series)
	r.Get("/api/v1/logs/tail", h.Tail)
	return r
}

func TestLogsQueryRangeProxies(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query_range" {
			t.Errorf("upstream path=%s", r.URL.Path)
		}
		if r.URL.Query().Get("query") != `{job="x"}` {
			t.Errorf("query=%q", r.URL.Query().Get("query"))
		}
		_, _ = w.Write([]byte(`{"status":"success"}`))
	})
	r := newLogsTestServer(t, upstream)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", `/api/v1/logs/query?query=%7Bjob%3D%22x%22%7D`, nil))
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"success"`) {
		t.Errorf("body=%s", rr.Body.String())
	}
}

func TestLogsQueryRangeRejectsEmpty(t *testing.T) {
	r := newLogsTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/logs/query", nil))
	if rr.Code != 400 {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestLogsLabelValues(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/label/job/values" {
			t.Errorf("upstream path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":["a","b"]}`))
	})
	r := newLogsTestServer(t, upstream)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/logs/labels/job/values", nil))
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestLogsTailReturns501(t *testing.T) {
	r := newLogsTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/logs/tail?query=x", nil))
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("status=%d", rr.Code)
	}
}
