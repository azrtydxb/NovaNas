package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/host/protocolshare"
)

type fakeProtocolShareReader struct {
	list   []protocolshare.ProtocolShare
	detail *protocolshare.Detail
	getErr error
}

func (f *fakeProtocolShareReader) List(_ context.Context) ([]protocolshare.ProtocolShare, error) {
	return f.list, nil
}
func (f *fakeProtocolShareReader) Get(_ context.Context, _ protocolshare.ProtocolShare) (*protocolshare.Detail, error) {
	return f.detail, f.getErr
}

func TestProtocolShareList_Returns200(t *testing.T) {
	mgr := &fakeProtocolShareReader{list: []protocolshare.ProtocolShare{{Name: "data"}}}
	h := &ProtocolShareHandler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/protocol-shares", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []protocolshare.ProtocolShare
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Name != "data" {
		t.Errorf("got %+v", got)
	}
}

func TestProtocolShareGet_RejectsMissingQuery(t *testing.T) {
	mgr := &fakeProtocolShareReader{}
	h := &ProtocolShareHandler{Logger: newDiscardLogger(), Mgr: mgr}
	r := chi.NewRouter()
	r.Get("/api/v1/protocol-shares/{name}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/protocol-shares/data", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestProtocolShareGet_Returns200(t *testing.T) {
	mgr := &fakeProtocolShareReader{detail: &protocolshare.Detail{Path: "/tank/data"}}
	h := &ProtocolShareHandler{Logger: newDiscardLogger(), Mgr: mgr}
	r := chi.NewRouter()
	r.Get("/api/v1/protocol-shares/{name}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/protocol-shares/data?pool=tank&dataset=data", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestProtocolShareGet_HostError(t *testing.T) {
	mgr := &fakeProtocolShareReader{getErr: errors.New("boom")}
	h := &ProtocolShareHandler{Logger: newDiscardLogger(), Mgr: mgr}
	r := chi.NewRouter()
	r.Get("/api/v1/protocol-shares/{name}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/protocol-shares/data?pool=tank&dataset=data", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rr.Code)
	}
}
