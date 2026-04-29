package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/jobs"
)

type fakeACLReader struct {
	aces []dataset.ACE
	err  error
}

func (f *fakeACLReader) GetACL(_ context.Context, _ string) ([]dataset.ACE, error) {
	return f.aces, f.err
}

func TestDatasetACLGet_Returns200(t *testing.T) {
	mgr := &fakeACLReader{aces: []dataset.ACE{{Type: dataset.ACETypeAllow, Principal: "OWNER@", Permissions: []dataset.ACLPerm{dataset.PermFullControl}}}}
	h := &DatasetACLHandler{Logger: newDiscardLogger(), Dataset: mgr}
	r := chi.NewRouter()
	r.Get("/api/v1/datasets/{fullname}/acl", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets/tank%2Fdata/acl", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDatasetACLGet_NotSupported(t *testing.T) {
	mgr := &fakeACLReader{err: dataset.ErrACLNotSupported}
	h := &DatasetACLHandler{Logger: newDiscardLogger(), Dataset: mgr}
	r := chi.NewRouter()
	r.Get("/api/v1/datasets/{fullname}/acl", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets/tank%2Fdata/acl", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestDatasetACLSet_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &DatasetACLHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Put("/api/v1/datasets/{fullname}/acl", h.Set)
	body := `{"aces":[{"type":"allow","principal":"OWNER@","permissions":["full_control"]}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/tank%2Fdata/acl", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindDatasetSetACL {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p := disp.calls[0].Payload.(jobs.DatasetSetACLPayload)
	if p.Path != "/tank/data" {
		t.Errorf("path=%q", p.Path)
	}
}

func TestDatasetACLSet_RejectsEmpty(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &DatasetACLHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Put("/api/v1/datasets/{fullname}/acl", h.Set)
	body := `{"aces":[]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/tank%2Fdata/acl", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestDatasetACLAppend_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &DatasetACLHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/datasets/{fullname}/acl/append", h.Append)
	body := `{"ace":{"type":"allow","principal":"user:alice","permissions":["read"]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/tank%2Fdata/acl/append", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindDatasetAppendACE {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestDatasetACLRemove_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &DatasetACLHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/datasets/{fullname}/acl/{index}", h.Remove)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/datasets/tank%2Fdata/acl/0", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	p := disp.calls[0].Payload.(jobs.DatasetRemoveACEPayload)
	if p.Index != 0 || p.Path != "/tank/data" {
		t.Errorf("payload=%+v", p)
	}
}

func TestDatasetACLRemove_BadIndex(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &DatasetACLHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/datasets/{fullname}/acl/{index}", h.Remove)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/datasets/tank%2Fdata/acl/abc", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}
