package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newAlertsTestServer(t *testing.T, upstream http.Handler) (*AlertsHandler, *chi.Mux) {
	t.Helper()
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	h := &AlertsHandler{UpstreamURL: up.URL, HTTP: up.Client()}
	r := chi.NewRouter()
	r.Get("/api/v1/alerts", h.ListAlerts)
	r.Get("/api/v1/alerts/{fingerprint}", h.GetAlert)
	r.Get("/api/v1/alert-silences", h.ListSilences)
	r.Post("/api/v1/alert-silences", h.CreateSilence)
	r.Delete("/api/v1/alert-silences/{id}", h.DeleteSilence)
	r.Get("/api/v1/alert-receivers", h.ListReceivers)
	return h, r
}

func TestAlertsListProxiesUpstream(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/alerts" {
			t.Errorf("upstream path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"fingerprint":"abc","labels":{"alertname":"X"}}]`))
	})
	_, r := newAlertsTestServer(t, upstream)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/alerts", nil))
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"abc"`) {
		t.Errorf("body=%s", rr.Body.String())
	}
}

func TestAlertsGetByFingerprintFound(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"fingerprint":"a"},{"fingerprint":"b"}]`))
	})
	_, r := newAlertsTestServer(t, upstream)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/alerts/b", nil))
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got["fingerprint"] != "b" {
		t.Errorf("got=%v", got)
	}
}

func TestAlertsGetByFingerprintMiss(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	_, r := newAlertsTestServer(t, upstream)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/alerts/zzz", nil))
	if rr.Code != 404 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAlertsCreateSilenceForwardsBody(t *testing.T) {
	gotBody := ""
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v2/silences" {
			t.Errorf("upstream %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"silenceID":"s1"}`))
	})
	_, r := newAlertsTestServer(t, upstream)
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"matchers":[{"name":"alertname","value":"X","isRegex":false}]}`)
	r.ServeHTTP(rr, httptest.NewRequest("POST", "/api/v1/alert-silences", body))
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(gotBody, `"alertname"`) {
		t.Errorf("upstream body=%s", gotBody)
	}
}

func TestAlertsDeleteSilenceEscapesID(t *testing.T) {
	gotPath := ""
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(200)
	})
	_, r := newAlertsTestServer(t, upstream)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("DELETE", "/api/v1/alert-silences/abc-123", nil))
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	if gotPath != "/api/v2/silence/abc-123" {
		t.Errorf("upstream path=%s", gotPath)
	}
}

func TestAlertsBadGatewayWhenUpstreamDown(t *testing.T) {
	h := &AlertsHandler{UpstreamURL: "http://127.0.0.1:1"}
	r := chi.NewRouter()
	r.Get("/api/v1/alerts", h.ListAlerts)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/alerts", nil).WithContext(context.Background()))
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status=%d", rr.Code)
	}
}
