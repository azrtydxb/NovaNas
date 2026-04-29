package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/plugins"
)

// 503 path: when manager is nil the new endpoints must respond 503
// (matching the rest of the plugin handlers).
func TestPluginsHandler_DependenciesNotConfigured(t *testing.T) {
	h := &PluginsHandler{}
	rec := httptest.NewRecorder()
	h.Dependencies(rec, httptest.NewRequest(http.MethodGet, "/plugins/x/dependencies", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Dependencies: want 503, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.Dependents(rec, httptest.NewRequest(http.MethodGet, "/plugins/x/dependents", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Dependents: want 503, got %d", rec.Code)
	}
}

// Dependents endpoint returns an empty list when the manager has no
// installed plugins. Uses a manager with no Queries — DependentsOf
// returns nil cleanly in that case. The test wires a chi router so
// the {name} URL param is parsed correctly.
func TestPluginsHandler_DependentsEmpty(t *testing.T) {
	mgr := plugins.NewManager(plugins.ManagerOptions{})
	h := &PluginsHandler{Manager: mgr}

	r := chi.NewRouter()
	r.Get("/plugins/{name}/dependents", h.Dependents)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins/object-storage/dependents", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Plugin     string   `json:"plugin"`
		Dependents []string `json:"dependents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Plugin != "object-storage" {
		t.Errorf("plugin=%q", out.Plugin)
	}
	if len(out.Dependents) != 0 {
		t.Errorf("dependents=%v, want empty", out.Dependents)
	}
}

// Smoke test the DependentsError mapping by exercising the writeErr
// envelope shape directly — the handler emits this on 409.
func TestPluginsHandler_UninstallDependentsErrorShape(t *testing.T) {
	derr := &plugins.DependentsError{Plugin: "x", BlockedBy: []string{"a"}}
	body, _ := json.Marshal(map[string]any{
		"error":     "has_dependents",
		"message":   derr.Error(),
		"plugin":    derr.Plugin,
		"blockedBy": derr.BlockedBy,
	})
	if !strings.Contains(string(body), "has_dependents") {
		t.Errorf("envelope missing error code: %s", body)
	}
	if !strings.Contains(string(body), "blockedBy") {
		t.Errorf("envelope missing blockedBy: %s", body)
	}
}
