package novanas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_PreviewPlugin(t *testing.T) {
	c, srv := newPluginsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins/index/rustfs/manifest" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if r.URL.Query().Get("version") != "1.2.3" {
			t.Errorf("missing version query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PluginPreview{
			Manifest: map[string]interface{}{
				"apiVersion": "novanas.io/v1",
				"kind":       "Plugin",
				"metadata":   map[string]interface{}{"name": "rustfs", "version": "1.2.3"},
			},
			Permissions: PluginPermissions{
				WillCreate: []PluginProvisionedResource{
					{Kind: "dataset", What: "ZFS dataset tank/objects"},
				},
				WillMount: []string{"/api/v1/plugins/rustfs/admin/*"},
				WillOpen:  []string{},
				Scopes:    []string{"PermPluginsRead"},
				Category:  "storage",
			},
			TarballSHA256: "abc123",
		})
	})
	defer srv.Close()

	got, err := c.PreviewPlugin(context.Background(), "rustfs", "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if got.TarballSHA256 != "abc123" {
		t.Errorf("sha=%q", got.TarballSHA256)
	}
	if got.Permissions.Category != "storage" {
		t.Errorf("category=%q", got.Permissions.Category)
	}
	if len(got.Permissions.WillCreate) != 1 || got.Permissions.WillCreate[0].Kind != "dataset" {
		t.Errorf("willCreate=%+v", got.Permissions.WillCreate)
	}
	if len(got.Permissions.WillMount) != 1 {
		t.Errorf("willMount=%v", got.Permissions.WillMount)
	}
}

func TestClient_PreviewPlugin_Validation(t *testing.T) {
	c, srv := newPluginsTestServer(t, func(http.ResponseWriter, *http.Request) {
		t.Error("server should not be hit")
	})
	defer srv.Close()
	if _, err := c.PreviewPlugin(context.Background(), "", "1.0.0"); err == nil {
		t.Error("expected name-required validation error")
	}
	if _, err := c.PreviewPlugin(context.Background(), "rustfs", ""); err == nil {
		t.Error("expected version-required validation error")
	}
}

func TestClient_PreviewPlugin_ServerError(t *testing.T) {
	c, srv := newPluginsTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"signature_invalid","message":"bad sig"}`))
	})
	defer srv.Close()
	if _, err := c.PreviewPlugin(context.Background(), "rustfs", "1.2.3"); err == nil {
		t.Fatal("expected error from 422")
	}
}

// httptest helper ServeMux response

var _ = httptest.NewServer
