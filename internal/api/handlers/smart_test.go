package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/host/smart"
	"github.com/novanas/nova-nas/internal/jobs"
)

type fakeSmartReader struct {
	health *smart.Health
	err    error
}

func (f *fakeSmartReader) Get(_ context.Context, _ string) (*smart.Health, error) {
	return f.health, f.err
}

func TestSmartGet_Returns200(t *testing.T) {
	mgr := &fakeSmartReader{health: &smart.Health{DeviceName: "/dev/sda", OverallPassed: true}}
	h := &SmartHandler{Logger: newDiscardLogger(), Mgr: mgr}
	r := chi.NewRouter()
	r.Get("/api/v1/disks/{name}/smart", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks/sda/smart", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestSmartGet_RejectsBadName(t *testing.T) {
	mgr := &fakeSmartReader{}
	h := &SmartHandler{Logger: newDiscardLogger(), Mgr: mgr}
	r := chi.NewRouter()
	r.Get("/api/v1/disks/{name}/smart", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks/sd-bad/smart", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestSmartRunSelfTest_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SmartHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/disks/{name}/smart/test", h.RunSelfTest)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/disks/nvme0n1/smart/test?type=short", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindSmartRunSelfTest {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p := disp.calls[0].Payload.(jobs.SmartRunSelfTestPayload)
	if p.DevicePath != "/dev/nvme0n1" || p.TestType != "short" {
		t.Errorf("payload=%+v", p)
	}
}

func TestSmartRunSelfTest_RejectsBadType(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &SmartHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/disks/{name}/smart/test", h.RunSelfTest)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/disks/sda/smart/test?type=bogus", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestSmartEnable_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SmartHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/disks/{name}/smart/enable", h.Enable)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/disks/sda/smart/enable", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	if disp.calls[0].Kind != jobs.KindSmartEnable {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}
