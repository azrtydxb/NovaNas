package novanas

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestClient_GetPluginDependencies(t *testing.T) {
	c, srv := newPluginsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins/rustfs/dependencies" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if r.URL.Query().Get("version") != "1.2.3" {
			t.Errorf("missing version=1.2.3")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PluginDependenciesResponse{
			Tree: &PluginDependencyTreeNode{Name: "rustfs", Source: "tier-2"},
			Plan: []PluginPlanStep{
				{Name: "object-storage", Version: "1.0.0", Source: "tier-2", Action: "install"},
				{Name: "rustfs", Version: "1.2.3", Source: "tier-2", Action: "install"},
			},
		})
	})
	defer srv.Close()
	out, err := c.GetPluginDependencies(context.Background(), "rustfs", "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if out.Tree.Name != "rustfs" {
		t.Errorf("tree.name=%q", out.Tree.Name)
	}
	if len(out.Plan) != 2 {
		t.Errorf("plan=%+v", out.Plan)
	}
}

func TestClient_GetPluginDependents(t *testing.T) {
	c, srv := newPluginsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins/object-storage/dependents" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PluginDependentsResponse{
			Plugin: "object-storage", Dependents: []string{"rustfs", "minio"},
		})
	})
	defer srv.Close()
	out, err := c.GetPluginDependents(context.Background(), "object-storage")
	if err != nil {
		t.Fatal(err)
	}
	if out.Plugin != "object-storage" || len(out.Dependents) != 2 {
		t.Errorf("got=%+v", out)
	}
}

func TestClient_UninstallPluginForce(t *testing.T) {
	c, srv := newPluginsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method=%s", r.Method)
		}
		if r.URL.Query().Get("purge") != "true" {
			t.Errorf("missing purge=true")
		}
		if r.URL.Query().Get("force") != "true" {
			t.Errorf("missing force=true")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()
	if err := c.UninstallPluginForce(context.Background(), "rustfs", true, true); err != nil {
		t.Fatal(err)
	}
}

func TestClient_GetPluginDependencies_RequiresName(t *testing.T) {
	c, _ := New(Config{BaseURL: "http://localhost"})
	if _, err := c.GetPluginDependencies(context.Background(), "", ""); err == nil {
		t.Fatal("expected error for empty name")
	}
}
