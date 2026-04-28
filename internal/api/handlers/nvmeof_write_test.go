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

func TestNvmeofCreateSubsystem_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"nqn":"nqn.2024-01.io.example:tank","allowAnyHost":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvmeof/subsystems", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CreateSubsystem(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindNvmeofSubsystemCreate {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestNvmeofCreateSubsystem_RejectsBadNQN(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvmeof/subsystems", bytes.NewBufferString(`{"nqn":"not-an-nqn"}`))
	rr := httptest.NewRecorder()
	h.CreateSubsystem(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("dispatched on bad nqn")
	}
}

func TestNvmeofDestroySubsystem_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/nvmeof/subsystems/{nqn}", h.DestroySubsystem)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/nvmeof/subsystems/nqn.2024-01.io.example:tank", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNvmeofAddNamespace_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/nvmeof/subsystems/{nqn}/namespaces", h.AddNamespace)
	body := `{"nsid":1,"devicePath":"/dev/zvol/tank/vol1","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvmeof/subsystems/nqn.2024-01.io.example:tank/namespaces", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNvmeofAddNamespace_RejectsBadDevicePath(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/nvmeof/subsystems/{nqn}/namespaces", h.AddNamespace)
	body := `{"nsid":1,"devicePath":"not-a-path"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvmeof/subsystems/nqn.2024-01.io.example:tank/namespaces", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestNvmeofRemoveNamespace_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/nvmeof/subsystems/{nqn}/namespaces/{nsid}", h.RemoveNamespace)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/nvmeof/subsystems/nqn.2024-01.io.example:tank/namespaces/1", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNvmeofAllowHost_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/nvmeof/subsystems/{nqn}/hosts", h.AllowHost)
	body := `{"hostNqn":"nqn.2024-01.io.example:host1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvmeof/subsystems/nqn.2024-01.io.example:tank/hosts", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNvmeofDisallowHost_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/nvmeof/subsystems/{nqn}/hosts/{hostNqn}", h.DisallowHost)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/nvmeof/subsystems/nqn.2024-01.io.example:tank/hosts/nqn.2024-01.io.example:host1", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNvmeofCreatePort_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"id":1,"ip":"10.0.0.1","port":4420,"transport":"tcp"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvmeof/ports", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CreatePort(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNvmeofCreatePort_RejectsBadTransport(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"id":1,"ip":"10.0.0.1","port":4420,"transport":"weird"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvmeof/ports", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CreatePort(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestNvmeofDeletePort_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/nvmeof/ports/{id}", h.DeletePort)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/nvmeof/ports/1", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNvmeofLinkSubsystem_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/nvmeof/ports/{id}/subsystems", h.LinkSubsystem)
	body := `{"nqn":"nqn.2024-01.io.example:tank"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvmeof/ports/1/subsystems", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNvmeofUnlinkSubsystem_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/nvmeof/ports/{id}/subsystems/{nqn}", h.UnlinkSubsystem)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/nvmeof/ports/1/subsystems/nqn.2024-01.io.example:tank", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
