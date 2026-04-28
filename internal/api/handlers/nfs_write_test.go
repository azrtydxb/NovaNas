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

func TestNfsCreateExport_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NfsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"name":"share1","path":"/tank/share1","clients":[{"spec":"10.0.0.0/24","options":"rw,sync"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nfs/exports", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CreateExport(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(disp.calls) != 1 || disp.calls[0].Kind != jobs.KindNfsExportCreate {
		t.Errorf("dispatch=%+v", disp.calls)
	}
	p := disp.calls[0].Payload.(jobs.NfsExportCreatePayload)
	if p.Export.Name != "share1" || p.Export.Path != "/tank/share1" || len(p.Export.Clients) != 1 {
		t.Errorf("payload=%+v", p)
	}
}

func TestNfsCreateExport_RejectsMissingClients(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &NfsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"name":"share1","path":"/tank/s1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nfs/exports", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CreateExport(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch")
	}
}

func TestNfsUpdateExport_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NfsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Patch("/api/v1/nfs/exports/{name}", h.UpdateExport)
	body := `{"name":"share1","path":"/tank/share1","clients":[{"spec":"*","options":"ro"}]}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/nfs/exports/share1", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindNfsExportUpdate {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestNfsUpdateExport_NameMismatch(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &NfsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Patch("/api/v1/nfs/exports/{name}", h.UpdateExport)
	body := `{"name":"different","path":"/tank/s1","clients":[{"spec":"*","options":"ro"}]}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/nfs/exports/share1", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestNfsDeleteExport_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NfsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/nfs/exports/{name}", h.DeleteExport)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/nfs/exports/share1", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	if disp.calls[0].Kind != jobs.KindNfsExportDelete {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p := disp.calls[0].Payload.(jobs.NfsExportDeletePayload)
	if p.Name != "share1" {
		t.Errorf("payload=%+v", p)
	}
}

func TestNfsReload_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NfsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nfs/reload", nil)
	rr := httptest.NewRecorder()
	h.Reload(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	if disp.calls[0].Kind != jobs.KindNfsReload {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}
