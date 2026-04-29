package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/novanas/nova-nas/internal/plugins"
)

// 503 path: when manager is nil, every endpoint must return 503.
func TestPluginsHandler_NotConfigured(t *testing.T) {
	h := &PluginsHandler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/plugins", nil)
	h.List(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("List: want 503, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.Install(rec, httptest.NewRequest(http.MethodPost, "/plugins", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Install: want 503, got %d", rec.Code)
	}
}

// Index 503 when marketplace not configured.
func TestPluginsHandler_IndexNotConfigured(t *testing.T) {
	h := &PluginsHandler{}
	rec := httptest.NewRecorder()
	h.Index(rec, httptest.NewRequest(http.MethodGet, "/plugins/index", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Index: want 503, got %d", rec.Code)
	}
}

// Install with bad JSON body returns 400.
func TestPluginsHandler_BadInstallBody(t *testing.T) {
	mgr := plugins.NewManager(plugins.ManagerOptions{})
	h := &PluginsHandler{Manager: mgr}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/plugins", nil)
	req.Body = http.NoBody
	h.Install(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env struct{ Error string `json:"error"` }
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error == "" {
		t.Errorf("expected error envelope, got %s", rec.Body.String())
	}
}
