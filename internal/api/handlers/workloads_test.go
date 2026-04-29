package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/workloads"
)

// fakeLifecycle satisfies workloads.Lifecycle.
type fakeLifecycle struct {
	indexEntries []workloads.IndexEntry
	indexEntry   *workloads.IndexEntryDetail
	indexErr     error

	releases []workloads.Release
	detail   *workloads.ReleaseDetail
	getErr   error

	installRel    *workloads.Release
	installErr    error
	installCalled workloads.InstallRequest

	upgradeRel    *workloads.Release
	upgradeErr    error
	upgradeCalled workloads.UpgradeRequest

	uninstallErr error

	rollbackRel *workloads.Release
	rollbackErr error

	events []workloads.Event
	logs   io.ReadCloser
	logErr error
}

func (f *fakeLifecycle) IndexList(_ context.Context) ([]workloads.IndexEntry, error) {
	return f.indexEntries, f.indexErr
}
func (f *fakeLifecycle) IndexGet(_ context.Context, _ string) (*workloads.IndexEntryDetail, error) {
	return f.indexEntry, f.indexErr
}
func (f *fakeLifecycle) IndexReload(_ context.Context) error { return f.indexErr }

func (f *fakeLifecycle) List(_ context.Context) ([]workloads.Release, error) {
	return f.releases, nil
}
func (f *fakeLifecycle) Get(_ context.Context, _ string) (*workloads.ReleaseDetail, error) {
	return f.detail, f.getErr
}
func (f *fakeLifecycle) Install(_ context.Context, req workloads.InstallRequest) (*workloads.Release, error) {
	f.installCalled = req
	return f.installRel, f.installErr
}
func (f *fakeLifecycle) Upgrade(_ context.Context, _ string, req workloads.UpgradeRequest) (*workloads.Release, error) {
	f.upgradeCalled = req
	return f.upgradeRel, f.upgradeErr
}
func (f *fakeLifecycle) Uninstall(_ context.Context, _ string) error { return f.uninstallErr }
func (f *fakeLifecycle) Rollback(_ context.Context, _ string, _ int) (*workloads.Release, error) {
	return f.rollbackRel, f.rollbackErr
}
func (f *fakeLifecycle) Events(_ context.Context, _ string) ([]workloads.Event, error) {
	return f.events, nil
}
func (f *fakeLifecycle) Logs(_ context.Context, _ string, _ workloads.LogRequest) (io.ReadCloser, error) {
	return f.logs, f.logErr
}

func newWorkloadsRouter(h *WorkloadsHandler) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/v1/workloads/index", h.ListIndex)
	r.Get("/api/v1/workloads/index/{name}", h.GetIndexEntry)
	r.Post("/api/v1/workloads/index/reload", h.ReloadIndex)
	r.Get("/api/v1/workloads", h.List)
	r.Post("/api/v1/workloads", h.Install)
	r.Get("/api/v1/workloads/{releaseName}", h.Get)
	r.Patch("/api/v1/workloads/{releaseName}", h.Upgrade)
	r.Delete("/api/v1/workloads/{releaseName}", h.Uninstall)
	r.Post("/api/v1/workloads/{releaseName}/rollback", h.Rollback)
	r.Get("/api/v1/workloads/{releaseName}/events", h.Events)
	r.Get("/api/v1/workloads/{releaseName}/logs", h.Logs)
	return r
}

func TestWorkloads_NotConfiguredReturns503(t *testing.T) {
	h := &WorkloadsHandler{Logger: newDiscardLogger()}
	r := newWorkloadsRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/index", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestWorkloads_ListIndex(t *testing.T) {
	lc := &fakeLifecycle{indexEntries: []workloads.IndexEntry{{Name: "plex"}}}
	h := &WorkloadsHandler{Logger: newDiscardLogger(), Lifecycle: lc}
	r := newWorkloadsRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/index", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got []workloads.IndexEntry
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Name != "plex" {
		t.Errorf("body=%+v", got)
	}
}

func TestWorkloads_ListIndex_Empty(t *testing.T) {
	lc := &fakeLifecycle{}
	h := &WorkloadsHandler{Logger: newDiscardLogger(), Lifecycle: lc}
	r := newWorkloadsRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/index", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if !strings.HasPrefix(rr.Body.String(), "[]") {
		t.Errorf("want [], got %q", rr.Body.String())
	}
}

func TestWorkloads_GetIndexEntry_NotFound(t *testing.T) {
	lc := &fakeLifecycle{indexErr: workloads.ErrNotFound}
	h := &WorkloadsHandler{Logger: newDiscardLogger(), Lifecycle: lc}
	r := newWorkloadsRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/index/missing", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestWorkloads_Install(t *testing.T) {
	lc := &fakeLifecycle{installRel: &workloads.Release{Name: "plex", Namespace: "nova-app-plex"}}
	h := &WorkloadsHandler{Logger: newDiscardLogger(), Lifecycle: lc}
	r := newWorkloadsRouter(h)
	body, _ := json.Marshal(workloads.InstallRequest{IndexName: "plex", ReleaseName: "plex"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workloads", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if lc.installCalled.IndexName != "plex" {
		t.Errorf("call=%+v", lc.installCalled)
	}
}

func TestWorkloads_Install_AlreadyExists(t *testing.T) {
	lc := &fakeLifecycle{installErr: workloads.ErrAlreadyExists}
	h := &WorkloadsHandler{Logger: newDiscardLogger(), Lifecycle: lc}
	r := newWorkloadsRouter(h)
	body, _ := json.Marshal(workloads.InstallRequest{IndexName: "plex", ReleaseName: "plex"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workloads", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestWorkloads_Install_Invalid(t *testing.T) {
	lc := &fakeLifecycle{installErr: errors.New("workloads: invalid argument: x")}
	// Wrap with errors.Is by using the actual sentinel:
	lc.installErr = workloads.ErrInvalidArgument
	h := &WorkloadsHandler{Logger: newDiscardLogger(), Lifecycle: lc}
	r := newWorkloadsRouter(h)
	body, _ := json.Marshal(workloads.InstallRequest{IndexName: "plex"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workloads", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestWorkloads_Upgrade(t *testing.T) {
	lc := &fakeLifecycle{upgradeRel: &workloads.Release{Name: "plex", Revision: 2}}
	h := &WorkloadsHandler{Logger: newDiscardLogger(), Lifecycle: lc}
	r := newWorkloadsRouter(h)
	body, _ := json.Marshal(workloads.UpgradeRequest{Version: "9.4.8"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/workloads/plex", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if lc.upgradeCalled.Version != "9.4.8" {
		t.Errorf("call=%+v", lc.upgradeCalled)
	}
}

func TestWorkloads_Uninstall(t *testing.T) {
	lc := &fakeLifecycle{}
	h := &WorkloadsHandler{Logger: newDiscardLogger(), Lifecycle: lc}
	r := newWorkloadsRouter(h)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/workloads/plex", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestWorkloads_Rollback(t *testing.T) {
	lc := &fakeLifecycle{rollbackRel: &workloads.Release{Name: "plex", Revision: 1}}
	h := &WorkloadsHandler{Logger: newDiscardLogger(), Lifecycle: lc}
	r := newWorkloadsRouter(h)
	body, _ := json.Marshal(map[string]int{"revision": 1})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workloads/plex/rollback", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestWorkloads_Logs(t *testing.T) {
	lc := &fakeLifecycle{logs: io.NopCloser(strings.NewReader("hello\nworld\n"))}
	h := &WorkloadsHandler{Logger: newDiscardLogger(), Lifecycle: lc}
	r := newWorkloadsRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/plex/logs?tail=10", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "hello") {
		t.Errorf("body=%q", rr.Body.String())
	}
}

func TestWorkloads_Logs_NoCluster(t *testing.T) {
	lc := &fakeLifecycle{logErr: workloads.ErrNoCluster}
	h := &WorkloadsHandler{Logger: newDiscardLogger(), Lifecycle: lc}
	r := newWorkloadsRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/plex/logs", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status=%d", rr.Code)
	}
}
