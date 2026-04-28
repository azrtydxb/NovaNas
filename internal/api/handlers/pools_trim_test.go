package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/jobs"
)

func TestPoolsTrim_Start(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/trim", h.Trim)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/trim", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindPoolTrim {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.PoolTrimPayload)
	if !ok || p.Name != "tank" || p.Action != "start" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

func TestPoolsTrim_StopWithDisk(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/trim", h.Trim)
	body := `{"disk":"/dev/sda"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/trim?action=stop", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	p := disp.calls[0].Payload.(jobs.PoolTrimPayload)
	if p.Action != "stop" || p.Disk != "/dev/sda" {
		t.Errorf("payload=%+v", p)
	}
}

func TestPoolsTrim_BadActionRejected(t *testing.T) {
	disp := &fakeDispatcher{}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/trim", h.Trim)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/trim?action=garbage", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestPoolsSetProps_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPatch, "/api/v1/pools/{name}/properties", h.SetProps)
	body := `{"properties":{"autotrim":"on"}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/pools/tank/properties", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindPoolSetProps {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.PoolSetPropsPayload)
	if !ok || p.Name != "tank" || p.Properties["autotrim"] != "on" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

func TestPoolsSetProps_RejectsEmpty(t *testing.T) {
	disp := &fakeDispatcher{}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPatch, "/api/v1/pools/{name}/properties", h.SetProps)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/pools/tank/properties", bytes.NewBufferString(`{"properties":{}}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}
