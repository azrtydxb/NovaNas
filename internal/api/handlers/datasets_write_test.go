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

func TestDatasetsCreate_202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &DatasetsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"parent":"tank","name":"home","type":"filesystem","properties":{"compression":"lz4"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindDatasetCreate {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestDatasetsCreate_BadName(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &DatasetsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"parent":"tank","name":"bad@name","type":"filesystem"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch")
	}
}

func TestDatasetsDestroy_URLEncoded(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &DatasetsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/datasets/{fullname}", h.Destroy)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home") + "?recursive=true"
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Errorf("status=%d", rr.Code)
	}
	p, ok := disp.calls[0].Payload.(jobs.DatasetDestroyPayload)
	if !ok || !p.Recursive {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

func TestDatasetsSetProps_PATCH(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &DatasetsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Patch("/api/v1/datasets/{fullname}", h.SetProps)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home")
	body := `{"properties":{"compression":"zstd"}}`
	req := httptest.NewRequest(http.MethodPatch, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindDatasetSet {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestDatasetsSetProps_RejectsEmpty(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &DatasetsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	r := chi.NewRouter()
	r.Patch("/api/v1/datasets/{fullname}", h.SetProps)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home")
	body := `{"properties":{}}`
	req := httptest.NewRequest(http.MethodPatch, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestDatasetsCreate_Duplicate409(t *testing.T) {
	disp := &fakeDispatcher{err: jobs.ErrDuplicate}
	h := &DatasetsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"parent":"tank","name":"home","type":"filesystem"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusConflict {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestDatasetsCreate_DispatchErrorReturns500(t *testing.T) {
	disp := &fakeDispatcher{err: errors.New("boom")}
	h := &DatasetsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}
	body := `{"parent":"tank","name":"home","type":"filesystem"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets", bytes.NewBufferString(body))
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
