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

func TestIscsiCreateTarget_Returns202(t *testing.T) {
	id := uuid.New()
	disp := &fakeDispatcher{out: id}
	h := &IscsiWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	body := `{"iqn":"iqn.2024-01.io.example:tank"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iscsi/targets", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CreateTarget(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(disp.calls) != 1 || disp.calls[0].Kind != jobs.KindIscsiTargetCreate {
		t.Errorf("dispatch=%+v", disp.calls)
	}
	p := disp.calls[0].Payload.(jobs.IscsiTargetCreatePayload)
	if p.IQN != "iqn.2024-01.io.example:tank" {
		t.Errorf("payload=%+v", p)
	}
}

func TestIscsiCreateTarget_RejectsBadIQN(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &IscsiWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iscsi/targets", bytes.NewBufferString(`{"iqn":"not-an-iqn"}`))
	rr := httptest.NewRecorder()
	h.CreateTarget(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch on bad iqn")
	}
}

func TestIscsiDestroyTarget_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &IscsiWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Delete("/api/v1/iscsi/targets/{iqn}", h.DestroyTarget)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/iscsi/targets/iqn.2024-01.io.example:tank", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindIscsiTargetDestroy {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestIscsiCreatePortal_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &IscsiWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Post("/api/v1/iscsi/targets/{iqn}/portals", h.CreatePortal)
	body := `{"ip":"192.168.1.10","port":3260,"transport":"tcp"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iscsi/targets/iqn.2024-01.io.example:tank/portals", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindIscsiPortalCreate {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestIscsiCreatePortal_RejectsBadIP(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &IscsiWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Post("/api/v1/iscsi/targets/{iqn}/portals", h.CreatePortal)
	body := `{"ip":"nope","port":3260}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iscsi/targets/iqn.2024-01.io.example:tank/portals", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestIscsiDeletePortal_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &IscsiWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Delete("/api/v1/iscsi/targets/{iqn}/portals/{ip}/{port}", h.DeletePortal)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/iscsi/targets/iqn.2024-01.io.example:tank/portals/192.168.1.10/3260", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindIscsiPortalDelete {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestIscsiCreateLUN_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &IscsiWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Post("/api/v1/iscsi/targets/{iqn}/luns", h.CreateLUN)
	body := `{"id":0,"backstore":"tank-vol1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iscsi/targets/iqn.2024-01.io.example:tank/luns", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestIscsiCreateLUN_RejectsNegativeID(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &IscsiWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Post("/api/v1/iscsi/targets/{iqn}/luns", h.CreateLUN)
	body := `{"id":-1,"backstore":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iscsi/targets/iqn.2024-01.io.example:tank/luns", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestIscsiDeleteLUN_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &IscsiWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Delete("/api/v1/iscsi/targets/{iqn}/luns/{id}", h.DeleteLUN)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/iscsi/targets/iqn.2024-01.io.example:tank/luns/0", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestIscsiCreateACL_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &IscsiWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Post("/api/v1/iscsi/targets/{iqn}/acls", h.CreateACL)
	body := `{"initiatorIqn":"iqn.2024-01.io.example:host1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iscsi/targets/iqn.2024-01.io.example:tank/acls", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestIscsiCreateACL_RejectsBadCHAPSecret(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &IscsiWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Post("/api/v1/iscsi/targets/{iqn}/acls", h.CreateACL)
	body := `{"initiatorIqn":"iqn.2024-01.io.example:host1","chapUser":"u","chapSecret":"short"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iscsi/targets/iqn.2024-01.io.example:tank/acls", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestIscsiDeleteACL_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &IscsiWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Delete("/api/v1/iscsi/targets/{iqn}/acls/{initiatorIqn}", h.DeleteACL)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/iscsi/targets/iqn.2024-01.io.example:tank/acls/iqn.2024-01.io.example:host1", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestIscsiSaveConfig_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &IscsiWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iscsi/saveconfig", nil)
	rr := httptest.NewRecorder()
	h.SaveConfig(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindIscsiSaveConfig {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}
