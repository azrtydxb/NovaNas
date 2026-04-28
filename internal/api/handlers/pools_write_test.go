package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/jobs"
)

type fakeDispatcher struct {
	calls []jobs.DispatchInput
	out   uuid.UUID
}

func (f *fakeDispatcher) Dispatch(_ context.Context, in jobs.DispatchInput) (jobs.DispatchOutput, error) {
	f.calls = append(f.calls, in)
	return jobs.DispatchOutput{JobID: f.out}, nil
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
