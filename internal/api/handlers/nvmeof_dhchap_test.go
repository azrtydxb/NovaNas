package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/host/configfs"
	"github.com/novanas/nova-nas/internal/host/nvmeof"
	"github.com/novanas/nova-nas/internal/jobs"
)

func TestNvmeofSetHostDHChap_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/nvmeof/hosts/{nqn}/dhchap", h.SetHostDHChap)

	body := `{"key":"DHHC-1:01:` + strings.Repeat("a", 30) + `","hash":"hmac(sha256)","dhgroup":"null"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/nvmeof/hosts/nqn.2024-01.io.example:client/dhchap",
		bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(disp.calls) != 1 || disp.calls[0].Kind != jobs.KindNvmeofSetHostDHChap {
		t.Errorf("dispatch=%+v", disp.calls)
	}
	p, ok := disp.calls[0].Payload.(jobs.NvmeofSetHostDHChapPayload)
	if !ok {
		t.Fatalf("payload type=%T", disp.calls[0].Payload)
	}
	if p.HostNQN != "nqn.2024-01.io.example:client" {
		t.Errorf("HostNQN=%q", p.HostNQN)
	}
	if p.Config.Hash != "hmac(sha256)" {
		t.Errorf("Hash=%q", p.Config.Hash)
	}
}

func TestNvmeofSetHostDHChap_RejectsBadNQN(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/nvmeof/hosts/{nqn}/dhchap", h.SetHostDHChap)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/nvmeof/hosts/not-an-nqn/dhchap",
		bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("dispatched on bad nqn")
	}
}

func TestNvmeofSetHostDHChap_PassesBadConfigThrough(t *testing.T) {
	// The handler defers config-content validation to the Manager. A
	// "bad hash" body should still 202 — failure surfaces as a failed
	// job, not a 400.
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/nvmeof/hosts/{nqn}/dhchap", h.SetHostDHChap)

	body := `{"hash":"bogus"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/nvmeof/hosts/nqn.2024-01.io.example:client/dhchap",
		bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNvmeofSetHostDHChap_RejectsBadJSON(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/nvmeof/hosts/{nqn}/dhchap", h.SetHostDHChap)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/nvmeof/hosts/nqn.2024-01.io.example:client/dhchap",
		bytes.NewBufferString(`not-json`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestNvmeofClearHostDHChap_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/nvmeof/hosts/{nqn}/dhchap", h.ClearHostDHChap)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/nvmeof/hosts/nqn.2024-01.io.example:client/dhchap", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindNvmeofClearHostDHChap {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.NvmeofClearHostDHChapPayload)
	if !ok || p.HostNQN != "nqn.2024-01.io.example:client" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

func TestNvmeofClearHostDHChap_RejectsBadNQN(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &NvmeofWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/nvmeof/hosts/{nqn}/dhchap", h.ClearHostDHChap)
	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/nvmeof/hosts/not-an-nqn/dhchap", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestNvmeofGetHostDHChap_OK(t *testing.T) {
	want := nvmeof.DHChapDetail{
		HasKey: true, HasCtrlKey: false, Hash: "hmac(sha256)", DHGroup: "ffdhe2048",
	}
	h := &NvmeofHandler{Logger: newDiscardLogger(), Mgr: &fakeNvmeofReader{dhchap: want}}
	r := chi.NewRouter()
	r.Get("/api/v1/nvmeof/hosts/{nqn}/dhchap", h.GetHostDHChap)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/nvmeof/hosts/nqn.2024-01.io.example:client/dhchap", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got nvmeof.DHChapDetail
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got=%+v want=%+v", got, want)
	}
}

func TestNvmeofGetHostDHChap_NotFound(t *testing.T) {
	h := &NvmeofHandler{Logger: newDiscardLogger(), Mgr: &fakeNvmeofReader{dhchapErr: configfs.ErrNotExist}}
	r := chi.NewRouter()
	r.Get("/api/v1/nvmeof/hosts/{nqn}/dhchap", h.GetHostDHChap)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/nvmeof/hosts/nqn.2024-01.io.example:gone/dhchap", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestNvmeofGetHostDHChap_BadNQN(t *testing.T) {
	h := &NvmeofHandler{Logger: newDiscardLogger(), Mgr: &fakeNvmeofReader{}}
	r := chi.NewRouter()
	r.Get("/api/v1/nvmeof/hosts/{nqn}/dhchap", h.GetHostDHChap)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/nvmeof/hosts/not-an-nqn/dhchap", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}
