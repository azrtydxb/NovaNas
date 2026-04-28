package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/host/nvmeof"
)

type fakeNvmeofReader struct {
	subs    []nvmeof.Subsystem
	subsErr error
	detail  *nvmeof.SubsystemDetail
	getErr  error
	ports   []nvmeof.Port
	portErr error
}

func (f *fakeNvmeofReader) ListSubsystems(_ context.Context) ([]nvmeof.Subsystem, error) {
	return f.subs, f.subsErr
}
func (f *fakeNvmeofReader) GetSubsystem(_ context.Context, _ string) (*nvmeof.SubsystemDetail, error) {
	return f.detail, f.getErr
}
func (f *fakeNvmeofReader) ListPorts(_ context.Context) ([]nvmeof.Port, error) {
	return f.ports, f.portErr
}

func TestNvmeofListSubsystems(t *testing.T) {
	h := &NvmeofHandler{Logger: newDiscardLogger(), Mgr: &fakeNvmeofReader{
		subs: []nvmeof.Subsystem{{NQN: "nqn.2024-01.io.example:tank"}},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvmeof/subsystems", nil)
	rr := httptest.NewRecorder()
	h.ListSubsystems(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []nvmeof.Subsystem
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 1 {
		t.Errorf("body=%+v", got)
	}
}

func TestNvmeofListSubsystems_HostError(t *testing.T) {
	h := &NvmeofHandler{Logger: newDiscardLogger(), Mgr: &fakeNvmeofReader{subsErr: errors.New("boom")}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvmeof/subsystems", nil)
	rr := httptest.NewRecorder()
	h.ListSubsystems(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestNvmeofGetSubsystem(t *testing.T) {
	h := &NvmeofHandler{Logger: newDiscardLogger(), Mgr: &fakeNvmeofReader{
		detail: &nvmeof.SubsystemDetail{Subsystem: nvmeof.Subsystem{NQN: "nqn.2024-01.io.example:tank"}},
	}}
	r := chi.NewRouter()
	r.Get("/api/v1/nvmeof/subsystems/{nqn}", h.GetSubsystem)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvmeof/subsystems/nqn.2024-01.io.example:tank", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestNvmeofGetSubsystem_EmptyNQN(t *testing.T) {
	// Direct call with no chi context — chi.URLParam returns "" — must 400.
	h := &NvmeofHandler{Logger: newDiscardLogger(), Mgr: &fakeNvmeofReader{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvmeof/subsystems/", nil)
	rr := httptest.NewRecorder()
	h.GetSubsystem(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestNvmeofListPorts(t *testing.T) {
	h := &NvmeofHandler{Logger: newDiscardLogger(), Mgr: &fakeNvmeofReader{
		ports: []nvmeof.Port{{ID: 1, IP: "10.0.0.1", Port: 4420, Transport: "tcp"}},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvmeof/ports", nil)
	rr := httptest.NewRecorder()
	h.ListPorts(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestNvmeofListPorts_EmptyReturnsArray(t *testing.T) {
	h := &NvmeofHandler{Logger: newDiscardLogger(), Mgr: &fakeNvmeofReader{ports: nil}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvmeof/ports", nil)
	rr := httptest.NewRecorder()
	h.ListPorts(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}
