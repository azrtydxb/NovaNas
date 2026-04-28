package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/jobs"
)

func TestSnapshotsCreate_202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SnapshotsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"dataset":"tank/home","name":"daily-1","recursive":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	p := disp.calls[0].Payload.(jobs.SnapshotCreatePayload)
	if !p.Recursive || p.Dataset != "tank/home" || p.ShortName != "daily-1" {
		t.Errorf("payload=%+v", p)
	}
}

func TestSnapshotsCreate_BadName(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &SnapshotsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"dataset":"tank/home","name":"-bad"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestSnapshotsDestroy_URLEncoded(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SnapshotsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/snapshots/{fullname}", h.Destroy)
	target := "/api/v1/snapshots/" + url.PathEscape("tank/home@snap1")
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindSnapshotDestroy {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestSnapshotsRollback(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SnapshotsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/datasets/{fullname}/rollback", h.Rollback)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home") + "/rollback"
	body := `{"snapshot":"daily-1"}`
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindSnapshotRollback {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p := disp.calls[0].Payload.(jobs.SnapshotRollbackPayload)
	if p.Snapshot != "tank/home@daily-1" {
		t.Errorf("snapshot=%q", p.Snapshot)
	}
}

func TestSnapshotsCreate_DispatchErrorReturns500(t *testing.T) {
	disp := &fakeDispatcher{err: errors.New("boom")}
	h := &SnapshotsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"dataset":"tank/home","name":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status=%d", rr.Code)
	}
	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&env)
	if env.Message == "boom" {
		t.Errorf("internal err leaked: %q", env.Message)
	}
}

func TestSnapshotsCreate_Duplicate409(t *testing.T) {
	disp := &fakeDispatcher{err: jobs.ErrDuplicate}
	h := &SnapshotsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"dataset":"tank/home","name":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusConflict {
		t.Errorf("status=%d", rr.Code)
	}
}
