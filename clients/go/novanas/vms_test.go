package novanas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func vmTestServer(handler http.Handler) (*Client, func()) {
	srv := httptest.NewServer(handler)
	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	return c, srv.Close
}

func TestSDK_ListVMs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/vms", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageSize") != "5" {
			t.Fatalf("pageSize: %s", r.URL.Query().Get("pageSize"))
		}
		_ = json.NewEncoder(w).Encode(VMPage{Items: []VM{{Name: "alpha", Namespace: "vm-alpha"}}, NextCursor: "vm-alpha/alpha"})
	})
	c, stop := vmTestServer(mux)
	defer stop()

	page, err := c.ListVMs(context.Background(), "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "alpha" {
		t.Fatalf("page: %+v", page)
	}
	if page.NextCursor != "vm-alpha/alpha" {
		t.Fatalf("cursor: %q", page.NextCursor)
	}
}

func TestSDK_CreateVM(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/vms", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method: %s", r.Method)
		}
		var req VMCreateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Name != "x" || req.TemplateID != "debian-12-cloud" {
			t.Fatalf("req: %+v", req)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(VM{Name: "x", Namespace: "vm-x", CPU: 2, MemoryMB: 2048})
	})
	c, stop := vmTestServer(mux)
	defer stop()
	vm, err := c.CreateVM(context.Background(), VMCreateRequest{Name: "x", TemplateID: "debian-12-cloud"})
	if err != nil {
		t.Fatal(err)
	}
	if vm.CPU != 2 {
		t.Fatalf("vm: %+v", vm)
	}
}

func TestSDK_StartStopMigrate(t *testing.T) {
	mux := http.NewServeMux()
	hits := map[string]int{}
	for _, p := range []string{"start", "stop", "migrate"} {
		p := p
		mux.HandleFunc("/api/v1/vms/vm-x/x/"+p, func(w http.ResponseWriter, _ *http.Request) {
			hits[p]++
			if p == "migrate" {
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = w.Write([]byte(`{"error":"not_implemented","message":"single node"}`))
				return
			}
			w.WriteHeader(http.StatusAccepted)
		})
	}
	c, stop := vmTestServer(mux)
	defer stop()
	if err := c.StartVM(context.Background(), "vm-x", "x"); err != nil {
		t.Fatal(err)
	}
	if err := c.StopVM(context.Background(), "vm-x", "x"); err != nil {
		t.Fatal(err)
	}
	if err := c.MigrateVM(context.Background(), "vm-x", "x"); err == nil {
		t.Fatal("expected migrate error")
	}
	if hits["start"] != 1 || hits["stop"] != 1 || hits["migrate"] != 1 {
		t.Fatalf("hits: %+v", hits)
	}
}

func TestSDK_Console(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/vms/vm-x/x/console", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("kind") != "vnc" {
			t.Fatalf("kind: %s", r.URL.Query().Get("kind"))
		}
		_ = json.NewEncoder(w).Encode(VMConsoleSession{WSURL: "wss://x/y", Token: "tok", Kind: "vnc"})
	})
	c, stop := vmTestServer(mux)
	defer stop()
	cs, err := c.GetVMConsole(context.Background(), "vm-x", "x", "vnc")
	if err != nil {
		t.Fatal(err)
	}
	if cs.WSURL == "" || cs.Token == "" {
		t.Fatalf("cs: %+v", cs)
	}
}

func TestSDK_Templates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/vm-templates", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []VMTemplate{{ID: "debian-12-cloud", DisplayName: "Debian 12"}},
		})
	})
	c, stop := vmTestServer(mux)
	defer stop()
	ts, err := c.ListVMTemplates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 || ts[0].ID != "debian-12-cloud" {
		t.Fatalf("templates: %+v", ts)
	}
}

func TestSDK_SnapshotsRestores(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/vm-snapshots", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["vmName"] != "x" {
				t.Fatalf("body: %+v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(VMSnapshot{Namespace: "vm-x", Name: "s1", VMName: "x", ReadyToUse: true})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []VMSnapshot{{Name: "s1", Namespace: "vm-x", VMName: "x"}}})
	})
	mux.HandleFunc("/api/v1/vm-restores", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(VMRestore{Namespace: "vm-x", Name: "r1", VMName: "x", SnapshotName: "s1", Complete: true})
	})
	c, stop := vmTestServer(mux)
	defer stop()
	snaps, err := c.ListVMSnapshots(context.Background(), "vm-x")
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 {
		t.Fatalf("snaps: %+v", snaps)
	}
	s, err := c.CreateVMSnapshot(context.Background(), "vm-x", "s1", "x")
	if err != nil {
		t.Fatal(err)
	}
	if !s.ReadyToUse {
		t.Fatalf("s: %+v", s)
	}
	r, err := c.CreateVMRestore(context.Background(), "vm-x", "r1", "x", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Complete {
		t.Fatalf("r: %+v", r)
	}
}

func TestSDK_GetVM_RequiresArgs(t *testing.T) {
	c := &Client{BaseURL: "http://x"}
	if _, err := c.GetVM(context.Background(), "", ""); err == nil {
		t.Fatal("want error")
	}
	if _, err := c.CreateVM(context.Background(), VMCreateRequest{}); err == nil || !strings.Contains(err.Error(), "Name") {
		t.Fatalf("create empty: %v", err)
	}
}
