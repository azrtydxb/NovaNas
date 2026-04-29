package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/jobs"
)

func TestProtocolShareCreate_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &ProtocolShareWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"name":"data","pool":"tank","datasetName":"data","protocols":["nfs"],"acls":[{"type":"allow","principal":"OWNER@","permissions":["full_control"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/protocol-shares", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindProtocolShareCreate {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p := disp.calls[0].Payload.(jobs.ProtocolShareCreatePayload)
	if p.Share.Name != "data" || p.Share.Pool != "tank" {
		t.Errorf("payload=%+v", p)
	}
}

func TestProtocolShareCreate_RejectsMissingPool(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &ProtocolShareWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"name":"data"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/protocol-shares", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch")
	}
}

func TestProtocolShareUpdate_NameMismatch(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &ProtocolShareWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Patch("/api/v1/protocol-shares/{name}", h.Update)
	body := `{"name":"other","pool":"tank","datasetName":"data"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/protocol-shares/data", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestProtocolShareDelete_LightDelete(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &ProtocolShareWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/protocol-shares/{name}", h.Delete)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/protocol-shares/data", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	p := disp.calls[0].Payload.(jobs.ProtocolShareDeletePayload)
	if p.Pool != "" || p.DatasetName != "" {
		t.Errorf("expected empty pool/dataset, got %+v", p)
	}
}

func TestProtocolShareDelete_FullTeardown(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &ProtocolShareWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/protocol-shares/{name}", h.Delete)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/protocol-shares/data?pool=tank&dataset=data", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	p := disp.calls[0].Payload.(jobs.ProtocolShareDeletePayload)
	if p.Pool != "tank" || p.DatasetName != "data" {
		t.Errorf("expected pool/dataset set, got %+v", p)
	}
}

func TestProtocolShareDelete_RejectsPartialQuery(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &ProtocolShareWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/protocol-shares/{name}", h.Delete)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/protocol-shares/data?pool=tank", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}
