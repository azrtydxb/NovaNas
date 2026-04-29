package novanas

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c, err := New(Config{BaseURL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, srv
}

func TestListWorkloadIndex(t *testing.T) {
	c, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workloads/index" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("auth=%s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]WorkloadIndexEntry{{Name: "plex"}})
	})
	defer srv.Close()
	got, err := c.ListWorkloadIndex(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Name != "plex" {
		t.Errorf("got=%+v", got)
	}
}

func TestInstallWorkload(t *testing.T) {
	c, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		var body WorkloadInstallRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.IndexName != "plex" || body.ReleaseName != "plex" {
			t.Errorf("body=%+v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(WorkloadRelease{Name: "plex"})
	})
	defer srv.Close()
	got, err := c.InstallWorkload(context.Background(), WorkloadInstallRequest{IndexName: "plex", ReleaseName: "plex"})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if got.Name != "plex" {
		t.Errorf("got=%+v", got)
	}
}

func TestInstallWorkload_Validation(t *testing.T) {
	c, _ := New(Config{BaseURL: "http://x"})
	if _, err := c.InstallWorkload(context.Background(), WorkloadInstallRequest{}); err == nil {
		t.Errorf("want validation error")
	}
}

func TestUninstallWorkload(t *testing.T) {
	c, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method=%s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()
	if err := c.UninstallWorkload(context.Background(), "plex"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
}

func TestUpgradeWorkload(t *testing.T) {
	c, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method=%s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(WorkloadRelease{Name: "plex", Revision: 2})
	})
	defer srv.Close()
	got, err := c.UpgradeWorkload(context.Background(), "plex", WorkloadUpgradeRequest{Version: "9.4.8"})
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if got.Revision != 2 {
		t.Errorf("got=%+v", got)
	}
}

func TestRollbackWorkload(t *testing.T) {
	c, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]int
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["revision"] != 1 {
			t.Errorf("body=%+v", body)
		}
		_ = json.NewEncoder(w).Encode(WorkloadRelease{Name: "plex"})
	})
	defer srv.Close()
	if _, err := c.RollbackWorkload(context.Background(), "plex", 1); err != nil {
		t.Fatalf("rollback: %v", err)
	}
}

func TestWorkloadLogs(t *testing.T) {
	c, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "tail=10") {
			t.Errorf("query=%s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte("hello\nworld\n"))
	})
	defer srv.Close()
	rc, err := c.WorkloadLogs(context.Background(), "plex", WorkloadLogOptions{TailLines: 10})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	defer rc.Close()
	buf, _ := io.ReadAll(rc)
	if !strings.Contains(string(buf), "hello") {
		t.Errorf("body=%q", buf)
	}
}

func TestWorkloadLogs_Error(t *testing.T) {
	c, srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"no_cluster","message":"unavailable"}`))
	})
	defer srv.Close()
	if _, err := c.WorkloadLogs(context.Background(), "plex", WorkloadLogOptions{}); err == nil {
		t.Errorf("want error")
	}
}
