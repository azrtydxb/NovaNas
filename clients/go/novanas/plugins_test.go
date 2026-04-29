package novanas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newPluginsTestServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}
	return c, srv
}

func TestClient_ListPluginIndex(t *testing.T) {
	c, srv := newPluginsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins/index" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if r.URL.Query().Get("refresh") != "true" {
			t.Errorf("missing refresh=true")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PluginIndex{Version: 1, Plugins: []PluginIndexEntry{{Name: "rustfs"}}})
	})
	defer srv.Close()
	idx, err := c.ListPluginIndex(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Plugins) != 1 || idx.Plugins[0].Name != "rustfs" {
		t.Errorf("idx=%+v", idx)
	}
}

func TestClient_InstallPlugin(t *testing.T) {
	c, srv := newPluginsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(PluginInstallation{Name: "rustfs", Version: "1.2.3", Status: "installed"})
	})
	defer srv.Close()
	got, err := c.InstallPlugin(context.Background(), PluginInstallRequest{Name: "rustfs", Version: "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "rustfs" || got.Version != "1.2.3" {
		t.Errorf("got=%+v", got)
	}
}

func TestClient_UninstallPlugin_Purge(t *testing.T) {
	c, srv := newPluginsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method=%s", r.Method)
		}
		if !strings.Contains(r.URL.RawQuery, "purge=true") {
			t.Errorf("query=%s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()
	if err := c.UninstallPlugin(context.Background(), "rustfs", true); err != nil {
		t.Fatal(err)
	}
}

func TestClient_InstallPlugin_NameRequired(t *testing.T) {
	c, srv := newPluginsTestServer(t, func(http.ResponseWriter, *http.Request) {
		t.Error("server should not be hit")
	})
	defer srv.Close()
	if _, err := c.InstallPlugin(context.Background(), PluginInstallRequest{}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestClient_ListPluginIndexWithOptions(t *testing.T) {
	c, srv := newPluginsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins/index" {
			t.Errorf("path=%s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("displayCategory") != "storage" {
			t.Errorf("missing displayCategory: %s", r.URL.RawQuery)
		}
		tags := q["tag"]
		if len(tags) != 2 || tags[0] != "s3" || tags[1] != "object" {
			t.Errorf("tags=%+v", tags)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PluginIndex{
			Version: 1,
			Plugins: []PluginIndexEntry{{
				Name:            "object-storage",
				DisplayCategory: "storage",
				Tags:            []string{"s3", "object", "backup-target"},
			}},
		})
	})
	defer srv.Close()
	idx, err := c.ListPluginIndexWithOptions(context.Background(), ListPluginIndexOptions{
		DisplayCategory: "storage",
		Tags:            []string{"s3", "object"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Plugins) != 1 || idx.Plugins[0].DisplayCategory != "storage" {
		t.Errorf("idx=%+v", idx)
	}
	if len(idx.Plugins[0].Tags) != 3 {
		t.Errorf("tags=%+v", idx.Plugins[0].Tags)
	}
}

func TestClient_ListPluginCategories(t *testing.T) {
	c, srv := newPluginsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins/categories" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]PluginCategoryCount{
			{Category: "storage", DisplayName: "Storage", Count: 2},
			{Category: "photos", DisplayName: "Photos", Count: 1},
		})
	})
	defer srv.Close()
	got, err := c.ListPluginCategories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Category != "storage" || got[0].Count != 2 {
		t.Errorf("got=%+v", got)
	}
}
