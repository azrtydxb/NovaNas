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

func TestSambaCreateShare_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SambaWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"name":"data","path":"/tank/data","writable":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/samba/shares", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CreateShare(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindSambaShareCreate {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p := disp.calls[0].Payload.(jobs.SambaShareCreatePayload)
	if p.Share.Name != "data" || p.Share.Path != "/tank/data" {
		t.Errorf("payload=%+v", p)
	}
}

func TestSambaCreateShare_RejectsMissingPath(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &SambaWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"name":"data"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/samba/shares", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CreateShare(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch")
	}
}

func TestSambaUpdateShare_NameMismatch(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &SambaWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Patch("/api/v1/samba/shares/{name}", h.UpdateShare)
	body := `{"name":"other","path":"/tank/data"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/samba/shares/data", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestSambaDeleteShare_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SambaWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/samba/shares/{name}", h.DeleteShare)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/samba/shares/data", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	if disp.calls[0].Kind != jobs.KindSambaShareDelete {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestSambaReload_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SambaWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/samba/reload", nil)
	rr := httptest.NewRecorder()
	h.Reload(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestSambaAddUser_RejectsMissingPassword(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &SambaWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"username":"alice"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/samba/users", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.AddUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestSambaAddUser_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SambaWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"username":"alice","password":"hunter2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/samba/users", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.AddUser(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindSambaUserAdd {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestSambaSetUserPassword_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SambaWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Put("/api/v1/samba/users/{username}/password", h.SetUserPassword)
	body := `{"password":"newpw"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/samba/users/alice/password", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	p := disp.calls[0].Payload.(jobs.SambaUserSetPasswordPayload)
	if p.Username != "alice" || p.Password != "newpw" {
		t.Errorf("payload=%+v", p)
	}
}
