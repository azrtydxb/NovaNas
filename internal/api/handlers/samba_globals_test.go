package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/host/samba"
	"github.com/novanas/nova-nas/internal/jobs"
)

type fakeGlobalsReader struct {
	opts *samba.GlobalsOpts
	err  error
}

func (f *fakeGlobalsReader) GetGlobals(_ context.Context) (*samba.GlobalsOpts, error) {
	return f.opts, f.err
}

func TestSambaGlobalsGet_Returns200(t *testing.T) {
	mgr := &fakeGlobalsReader{opts: &samba.GlobalsOpts{Workgroup: "WORKGROUP"}}
	h := &SambaGlobalsHandler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/samba/globals", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestSambaGlobalsSet_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SambaGlobalsHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"workgroup":"WORKGROUP","aclProfile":"nfsv4","securityMode":"user"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/samba/globals", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Set(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindSambaSetGlobals {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p := disp.calls[0].Payload.(jobs.SambaSetGlobalsPayload)
	if p.Opts.Workgroup != "WORKGROUP" {
		t.Errorf("opts=%+v", p.Opts)
	}
}

func TestSambaGlobalsSet_RejectsBadJSON(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &SambaGlobalsHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/samba/globals", bytes.NewBufferString("not json"))
	rr := httptest.NewRecorder()
	h.Set(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}
