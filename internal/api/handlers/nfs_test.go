package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/host/nfs"
)

type fakeNfsReader struct {
	exports []nfs.Export
	get     *nfs.Export
	getErr  error
	active  []nfs.ActiveExport
}

func (f *fakeNfsReader) ListExports(_ context.Context) ([]nfs.Export, error) {
	return f.exports, nil
}
func (f *fakeNfsReader) GetExport(_ context.Context, _ string) (*nfs.Export, error) {
	return f.get, f.getErr
}
func (f *fakeNfsReader) ListActive(_ context.Context) ([]nfs.ActiveExport, error) {
	return f.active, nil
}

func TestNfsListExports_Returns200(t *testing.T) {
	mgr := &fakeNfsReader{exports: []nfs.Export{{Name: "share1", Path: "/tank/s1"}}}
	h := &NfsHandler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nfs/exports", nil)
	rr := httptest.NewRecorder()
	h.ListExports(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []nfs.Export
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Name != "share1" {
		t.Errorf("got %+v", got)
	}
}

func TestNfsGetExport_NotFound(t *testing.T) {
	mgr := &fakeNfsReader{getErr: nfs.ErrNotFound}
	h := &NfsHandler{Logger: newDiscardLogger(), Mgr: mgr}
	r := chi.NewRouter()
	r.Get("/api/v1/nfs/exports/{name}", h.GetExport)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nfs/exports/missing", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestNfsListActive_Returns200(t *testing.T) {
	mgr := &fakeNfsReader{active: []nfs.ActiveExport{{Path: "/tank/s1", Client: "10.0.0.0/24", Options: "rw"}}}
	h := &NfsHandler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nfs/exports/active", nil)
	rr := httptest.NewRecorder()
	h.ListActive(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}
