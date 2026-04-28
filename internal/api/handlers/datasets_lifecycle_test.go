package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/jobs"
)

func newDatasetsLifecycleHandler(disp *fakeDispatcher) *DatasetsWriteHandler {
	return &DatasetsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
}

func TestDatasetsRename_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newDatasetsLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/rename", h.Rename)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home") + "/rename"
	body := `{"newName":"tank/users","recursive":true}`
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindDatasetRename {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.DatasetRenamePayload)
	if !ok || p.OldName != "tank/home" || p.NewName != "tank/users" || !p.Recursive {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

func TestDatasetsRename_BadName400(t *testing.T) {
	disp := &fakeDispatcher{}
	h := newDatasetsLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/rename", h.Rename)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home") + "/rename"
	body := `{"newName":"bad@name","recursive":false}`
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch")
	}
}

func TestDatasetsClone_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newDatasetsLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/clone", h.Clone)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home@snap1") + "/clone"
	body := `{"target":"tank/clone","properties":{"compression":"zstd"}}`
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindDatasetClone {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.DatasetClonePayload)
	if !ok || p.Snapshot != "tank/home@snap1" || p.Target != "tank/clone" || p.Properties["compression"] != "zstd" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

func TestDatasetsPromote_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newDatasetsLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/promote", h.Promote)
	target := "/api/v1/datasets/" + url.PathEscape("tank/clone") + "/promote"
	req := httptest.NewRequest(http.MethodPost, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindDatasetPromote {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.DatasetPromotePayload)
	if !ok || p.Name != "tank/clone" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

func TestDatasetsLoadKey_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newDatasetsLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/load-key", h.LoadKey)
	target := "/api/v1/datasets/" + url.PathEscape("tank/secure") + "/load-key"
	body := `{"keylocation":"file:///etc/zfs/key","recursive":true}`
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	p, ok := disp.calls[0].Payload.(jobs.DatasetLoadKeyPayload)
	if !ok || p.Name != "tank/secure" || p.Keylocation != "file:///etc/zfs/key" || !p.Recursive {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

func TestDatasetsUnloadKey_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newDatasetsLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/unload-key", h.UnloadKey)
	target := "/api/v1/datasets/" + url.PathEscape("tank/secure") + "/unload-key"
	req := httptest.NewRequest(http.MethodPost, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindDatasetUnloadKey {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestDatasetsChangeKey_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newDatasetsLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/change-key", h.ChangeKey)
	target := "/api/v1/datasets/" + url.PathEscape("tank/secure") + "/change-key"
	body := `{"properties":{"keylocation":"prompt"}}`
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	p, ok := disp.calls[0].Payload.(jobs.DatasetChangeKeyPayload)
	if !ok || p.Name != "tank/secure" || p.Properties["keylocation"] != "prompt" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

func TestDatasetsChangeKey_RejectsEmpty(t *testing.T) {
	disp := &fakeDispatcher{}
	h := newDatasetsLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/change-key", h.ChangeKey)
	target := "/api/v1/datasets/" + url.PathEscape("tank/secure") + "/change-key"
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(`{"properties":{}}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}
