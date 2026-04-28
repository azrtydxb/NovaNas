package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/host/iscsi"
)

type fakeIscsiReader struct {
	list    []iscsi.Target
	listErr error
	detail  *iscsi.TargetDetail
	getErr  error
}

func (f *fakeIscsiReader) ListTargets(_ context.Context) ([]iscsi.Target, error) {
	return f.list, f.listErr
}
func (f *fakeIscsiReader) GetTarget(_ context.Context, _ string) (*iscsi.TargetDetail, error) {
	return f.detail, f.getErr
}

func TestIscsiList(t *testing.T) {
	h := &IscsiHandler{Logger: newDiscardLogger(), Mgr: &fakeIscsiReader{
		list: []iscsi.Target{{IQN: "iqn.2024-01.io.example:tank"}},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/iscsi/targets", nil)
	rr := httptest.NewRecorder()
	h.ListTargets(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []iscsi.Target
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 1 || got[0].IQN != "iqn.2024-01.io.example:tank" {
		t.Errorf("body=%+v", got)
	}
}

func TestIscsiList_EmptyReturnsArray(t *testing.T) {
	h := &IscsiHandler{Logger: newDiscardLogger(), Mgr: &fakeIscsiReader{list: nil}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/iscsi/targets", nil)
	rr := httptest.NewRecorder()
	h.ListTargets(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if rr.Body.String() == "null\n" || rr.Body.String() == "null" {
		t.Errorf("expected [] not null, got %q", rr.Body.String())
	}
}

func TestIscsiList_HostError(t *testing.T) {
	h := &IscsiHandler{Logger: newDiscardLogger(), Mgr: &fakeIscsiReader{listErr: errors.New("boom")}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/iscsi/targets", nil)
	rr := httptest.NewRecorder()
	h.ListTargets(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestIscsiGet(t *testing.T) {
	h := &IscsiHandler{Logger: newDiscardLogger(), Mgr: &fakeIscsiReader{
		detail: &iscsi.TargetDetail{Target: iscsi.Target{IQN: "iqn.2024-01.io.example:tank"}},
	}}
	r := chi.NewRouter()
	r.Get("/api/v1/iscsi/targets/{iqn}", h.GetTarget)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/iscsi/targets/iqn.2024-01.io.example:tank", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestIscsiGet_BadIqnEmpty(t *testing.T) {
	// Empty path param via direct call (chi router won't match empty).
	h := &IscsiHandler{Logger: newDiscardLogger(), Mgr: &fakeIscsiReader{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/iscsi/targets/", nil)
	rr := httptest.NewRecorder()
	h.GetTarget(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}
