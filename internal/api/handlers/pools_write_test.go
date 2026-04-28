package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/jobs"
)

type fakeDispatcher struct {
	calls []jobs.DispatchInput
	out   uuid.UUID
	err   error
}

func (f *fakeDispatcher) Dispatch(_ context.Context, in jobs.DispatchInput) (jobs.DispatchOutput, error) {
	f.calls = append(f.calls, in)
	return jobs.DispatchOutput{JobID: f.out}, f.err
}

func TestPoolsCreate_Returns202(t *testing.T) {
	id := uuid.New()
	disp := &fakeDispatcher{out: id}
	h := &PoolsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	body := `{"name":"tank","vdevs":[{"type":"mirror","disks":["/dev/A","/dev/B"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/api/v1/jobs/"+id.String() {
		t.Errorf("Location=%q", loc)
	}
	if len(disp.calls) != 1 || disp.calls[0].Kind != jobs.KindPoolCreate {
		t.Errorf("dispatch=%+v", disp.calls)
	}
}

func TestPoolsCreate_RejectsBadName(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &PoolsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"name":"bad/name","vdevs":[{"type":"mirror","disks":["/dev/A","/dev/B"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch on bad name")
	}
}

func TestPoolsCreate_RejectsBadJSON(t *testing.T) {
	h := &PoolsWriteHandler{Logger: newDiscardLogger(), Dispatcher: &fakeDispatcher{}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools", bytes.NewBufferString("not json"))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	_ = json.NewDecoder(rr.Body).Decode(&struct{}{})
}

func TestPoolsDestroy_Returns202(t *testing.T) {
	id := uuid.New()
	disp := &fakeDispatcher{out: id}
	h := &PoolsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Delete("/api/v1/pools/{name}", h.Destroy)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/pools/tank", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(disp.calls) != 1 || disp.calls[0].Kind != jobs.KindPoolDestroy {
		t.Errorf("kind=%v", disp.calls)
	}
	p, ok := disp.calls[0].Payload.(jobs.PoolDestroyPayload)
	if !ok || p.Name != "tank" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

func TestPoolsScrub_Start(t *testing.T) {
	id := uuid.New()
	disp := &fakeDispatcher{out: id}
	h := &PoolsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Post("/api/v1/pools/{name}/scrub", h.Scrub)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/scrub", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	if disp.calls[0].Kind != jobs.KindPoolScrub {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p := disp.calls[0].Payload.(jobs.PoolScrubPayload)
	if p.Action != pool.ScrubStart {
		t.Errorf("action=%q want start", p.Action)
	}
}

func TestPoolsScrub_Stop(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &PoolsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Post("/api/v1/pools/{name}/scrub", h.Scrub)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/scrub?action=stop", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	p := disp.calls[0].Payload.(jobs.PoolScrubPayload)
	if p.Action != pool.ScrubStop {
		t.Errorf("action=%q want stop", p.Action)
	}
}

func TestPoolsScrub_RejectsBadAction(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &PoolsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := chi.NewRouter()
	r.Post("/api/v1/pools/{name}/scrub", h.Scrub)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/scrub?action=garbage", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch on bad action")
	}
}

func TestPoolsCreate_DuplicateReturns409(t *testing.T) {
	disp := &fakeDispatcher{err: jobs.ErrDuplicate}
	h := &PoolsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	body := `{"name":"tank","vdevs":[{"type":"mirror","disks":["/dev/A","/dev/B"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var env struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&env)
	if env.Error != "duplicate" {
		t.Errorf("error=%q", env.Error)
	}
}

func TestPoolsCreate_DispatchErrorReturns500(t *testing.T) {
	disp := &fakeDispatcher{err: errors.New("redis down")}
	h := &PoolsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	body := `{"name":"tank","vdevs":[{"type":"mirror","disks":["/dev/A","/dev/B"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rr.Code)
	}
	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&env)
	if env.Error != "dispatch_error" {
		t.Errorf("error=%q", env.Error)
	}
	if env.Message == "redis down" {
		t.Errorf("internal err leaked: %q", env.Message)
	}
}
