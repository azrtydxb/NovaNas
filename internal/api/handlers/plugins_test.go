package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/novanas/nova-nas/internal/plugins"
)

// newFakeMarketplaceClient stands up an httptest server that serves the
// supplied index payload at any path, then returns a MarketplaceClient
// pinned to it. Caller must close the returned server.
func newFakeMarketplaceClient(t *testing.T, idx plugins.Index) (*plugins.MarketplaceClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(idx)
	}))
	c := plugins.NewMarketplaceClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
	return c, srv
}

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
	rec = httptest.NewRecorder()
	h.Restart(rec, httptest.NewRequest(http.MethodPost, "/plugins/x/restart", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Restart: want 503, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.Logs(rec, httptest.NewRequest(http.MethodGet, "/plugins/x/logs", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Logs: want 503, got %d", rec.Code)
	}
}

// Restart with no Queries wired: Manager.Restart returns ErrNotFound,
// handler maps that to 404.
func TestPluginsHandler_RestartNotFound(t *testing.T) {
	mgr := plugins.NewManager(plugins.ManagerOptions{})
	h := &PluginsHandler{Manager: mgr}
	rec := httptest.NewRecorder()
	h.Restart(rec, httptest.NewRequest(http.MethodPost, "/plugins/x/restart", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// Logs with bad lines query string returns 400 before touching the manager.
func TestPluginsHandler_LogsBadLines(t *testing.T) {
	mgr := plugins.NewManager(plugins.ManagerOptions{})
	h := &PluginsHandler{Manager: mgr}
	rec := httptest.NewRecorder()
	h.Logs(rec, httptest.NewRequest(http.MethodGet, "/plugins/x/logs?lines=abc", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.Logs(rec, httptest.NewRequest(http.MethodGet, "/plugins/x/logs?lines=-5", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for negative, got %d", rec.Code)
	}
}

// Logs with no Queries wired returns 404 (plugin not found).
func TestPluginsHandler_LogsNotFound(t *testing.T) {
	mgr := plugins.NewManager(plugins.ManagerOptions{})
	h := &PluginsHandler{Manager: mgr}
	rec := httptest.NewRecorder()
	h.Logs(rec, httptest.NewRequest(http.MethodGet, "/plugins/x/logs", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d body=%s", rec.Code, rec.Body.String())
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

// --- displayCategory + tags filtering ------------------------------------

func fixtureIndex() plugins.Index {
	return plugins.Index{
		Version: 1,
		Plugins: []plugins.IndexPlugin{
			{
				Name:            "object-storage",
				Vendor:          "novanas.io",
				Category:        "storage",
				DisplayCategory: "storage",
				Tags:            []string{"s3", "object", "backup-target"},
				Versions:        []plugins.IndexVersion{{Version: "1.0.0"}},
			},
			{
				Name:            "photo-album",
				Vendor:          "novanas.io",
				Category:        "utility",
				DisplayCategory: "photos",
				Tags:            []string{"photos", "album"},
				Versions:        []plugins.IndexVersion{{Version: "0.1.0"}},
			},
			{
				Name:            "another-storage",
				Vendor:          "novanas.io",
				Category:        "storage",
				DisplayCategory: "storage",
				Tags:            []string{"s3", "ceph"},
				Versions:        []plugins.IndexVersion{{Version: "2.0.0"}},
			},
		},
	}
}

func TestPluginsHandler_IndexFilterByDisplayCategory(t *testing.T) {
	mp, srv := newFakeMarketplaceClient(t, fixtureIndex())
	defer srv.Close()
	h := &PluginsHandler{Marketplace: mp}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/plugins/index?displayCategory=storage", nil)
	h.Index(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var got plugins.Index
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Plugins) != 2 {
		t.Fatalf("want 2 storage plugins, got %d: %+v", len(got.Plugins), got.Plugins)
	}
	for _, p := range got.Plugins {
		if p.DisplayCategory != "storage" {
			t.Errorf("unexpected: %+v", p)
		}
	}
}

func TestPluginsHandler_IndexFilterByDisplayCategoryUnknown(t *testing.T) {
	mp, srv := newFakeMarketplaceClient(t, fixtureIndex())
	defer srv.Close()
	h := &PluginsHandler{Marketplace: mp}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/plugins/index?displayCategory=bogus", nil)
	h.Index(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestPluginsHandler_IndexFilterByTagsAND(t *testing.T) {
	mp, srv := newFakeMarketplaceClient(t, fixtureIndex())
	defer srv.Close()
	h := &PluginsHandler{Marketplace: mp}
	rec := httptest.NewRecorder()
	// Both tags must match: only object-storage has BOTH s3 and object.
	req := httptest.NewRequest(http.MethodGet, "/plugins/index?tag=s3&tag=object", nil)
	h.Index(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var got plugins.Index
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Plugins) != 1 || got.Plugins[0].Name != "object-storage" {
		t.Fatalf("want only object-storage, got %+v", got.Plugins)
	}
}

func TestPluginsHandler_IndexFilterCombined(t *testing.T) {
	mp, srv := newFakeMarketplaceClient(t, fixtureIndex())
	defer srv.Close()
	h := &PluginsHandler{Marketplace: mp}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/plugins/index?displayCategory=storage&tag=ceph", nil)
	h.Index(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var got plugins.Index
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Plugins) != 1 || got.Plugins[0].Name != "another-storage" {
		t.Fatalf("want another-storage, got %+v", got.Plugins)
	}
}

func TestPluginsHandler_Categories(t *testing.T) {
	mp, srv := newFakeMarketplaceClient(t, fixtureIndex())
	defer srv.Close()
	h := &PluginsHandler{Marketplace: mp}
	rec := httptest.NewRecorder()
	h.Categories(rec, httptest.NewRequest(http.MethodGet, "/plugins/categories", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var got []struct {
		Category    string `json:"category"`
		DisplayName string `json:"displayName"`
		Count       int    `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(plugins.AllDisplayCategories) {
		t.Fatalf("want %d entries, got %d", len(plugins.AllDisplayCategories), len(got))
	}
	counts := map[string]int{}
	for _, e := range got {
		counts[e.Category] = e.Count
	}
	if counts["storage"] != 2 {
		t.Errorf("storage count: want 2, got %d", counts["storage"])
	}
	if counts["photos"] != 1 {
		t.Errorf("photos count: want 1, got %d", counts["photos"])
	}
	if counts["backup"] != 0 {
		t.Errorf("backup count: want 0, got %d", counts["backup"])
	}
}

func TestPluginsHandler_CategoriesNoMarketplace(t *testing.T) {
	h := &PluginsHandler{}
	rec := httptest.NewRecorder()
	h.Categories(rec, httptest.NewRequest(http.MethodGet, "/plugins/categories", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 (stable list with zero counts), got %d", rec.Code)
	}
	var got []struct {
		Category string `json:"category"`
		Count    int    `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(plugins.AllDisplayCategories) {
		t.Fatalf("want %d entries, got %d", len(plugins.AllDisplayCategories), len(got))
	}
}
