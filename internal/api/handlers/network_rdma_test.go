package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/novanas/nova-nas/internal/host/rdma"
)

func TestRDMA_List_NoHardware(t *testing.T) {
	// SysPath that doesn't exist returns empty array.
	h := &RDMAHandler{Logger: newDiscardLogger(), Lister: &rdma.Lister{SysPath: "/nope/does/not/exist"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/network/rdma", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got []rdma.Adapter
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got == nil {
		t.Errorf("expected [], got nil")
	}
}

func TestRDMA_List_NilLister(t *testing.T) {
	h := &RDMAHandler{Logger: newDiscardLogger(), Lister: nil}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/network/rdma", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestRDMA_List_WithAdapter(t *testing.T) {
	dir := t.TempDir()
	// Create a minimal /sys/class/infiniband-like layout.
	hca := filepath.Join(dir, "mlx5_0")
	if err := os.MkdirAll(filepath.Join(hca, "ports", "1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hca, "ports", "1", "state"), []byte("4: ACTIVE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hca, "ports", "1", "link_layer"), []byte("Ethernet\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &RDMAHandler{Logger: newDiscardLogger(), Lister: &rdma.Lister{SysPath: dir}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/network/rdma", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []rdma.Adapter
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 1 || got[0].Name != "mlx5_0" {
		t.Errorf("got=%+v", got)
	}
}
