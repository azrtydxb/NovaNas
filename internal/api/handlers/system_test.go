package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/host/system"
	"github.com/novanas/nova-nas/internal/jobs"
)

type fakeSystemReader struct {
	info *system.Info
	tc   *system.TimeConfig
	err  error
}

func (f *fakeSystemReader) GetInfo(_ context.Context) (*system.Info, error) {
	return f.info, f.err
}
func (f *fakeSystemReader) GetTimeConfig(_ context.Context) (*system.TimeConfig, error) {
	return f.tc, f.err
}

func TestSystemGetInfo_Returns200(t *testing.T) {
	mgr := &fakeSystemReader{info: &system.Info{Hostname: "nova"}}
	h := &SystemHandler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/info", nil)
	rr := httptest.NewRecorder()
	h.GetInfo(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestSystemGetTime_Returns200(t *testing.T) {
	mgr := &fakeSystemReader{tc: &system.TimeConfig{Timezone: "UTC", NTP: true}}
	h := &SystemHandler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/time", nil)
	rr := httptest.NewRecorder()
	h.GetTime(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestSystemSetHostname_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SystemHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"hostname":"nova-1"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/hostname", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.SetHostname(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindSystemSetHostname {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p := disp.calls[0].Payload.(jobs.SystemSetHostnamePayload)
	if p.Hostname != "nova-1" {
		t.Errorf("payload=%+v", p)
	}
}

func TestSystemSetHostname_Empty(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &SystemHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/hostname", bytes.NewBufferString(`{"hostname":""}`))
	rr := httptest.NewRecorder()
	h.SetHostname(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestSystemSetTimezone_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SystemHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"timezone":"Europe/Brussels"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/timezone", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.SetTimezone(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestSystemSetNTP_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SystemHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"enabled":true,"servers":["pool.ntp.org"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/ntp", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.SetNTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	p := disp.calls[0].Payload.(jobs.SystemSetNTPPayload)
	if !p.Enabled || len(p.Servers) != 1 {
		t.Errorf("payload=%+v", p)
	}
}

func TestSystemReboot_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SystemHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/reboot?delaySec=60", nil)
	rr := httptest.NewRecorder()
	h.Reboot(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	p := disp.calls[0].Payload.(jobs.SystemRebootPayload)
	if p.DelaySeconds != 60 {
		t.Errorf("delay=%d", p.DelaySeconds)
	}
}

func TestSystemReboot_BadDelay(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &SystemHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/reboot?delaySec=-1", nil)
	rr := httptest.NewRecorder()
	h.Reboot(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestSystemShutdown_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SystemHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/shutdown", nil)
	rr := httptest.NewRecorder()
	h.Shutdown(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestSystemCancelShutdown_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SystemHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/cancel-shutdown", nil)
	rr := httptest.NewRecorder()
	h.CancelShutdown(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
}
