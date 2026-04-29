package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/jobs"
	"github.com/novanas/nova-nas/internal/replication"
)

// fakeReplMgr is a hand-rolled stub for ReplicationManagerAPI.
type fakeReplMgr struct {
	jobs    map[uuid.UUID]replication.Job
	runs    map[uuid.UUID][]replication.Run
	createE error
	updateE error
	delE    error
}

func newFakeReplMgr() *fakeReplMgr {
	return &fakeReplMgr{
		jobs: map[uuid.UUID]replication.Job{},
		runs: map[uuid.UUID][]replication.Run{},
	}
}

func (f *fakeReplMgr) Create(_ context.Context, j replication.Job) (replication.Job, error) {
	if f.createE != nil {
		return replication.Job{}, f.createE
	}
	if err := j.Validate(); err != nil {
		return replication.Job{}, err
	}
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	f.jobs[j.ID] = j
	return j, nil
}

func (f *fakeReplMgr) Update(_ context.Context, j replication.Job) (replication.Job, error) {
	if f.updateE != nil {
		return replication.Job{}, f.updateE
	}
	if _, ok := f.jobs[j.ID]; !ok {
		return replication.Job{}, replication.ErrNotFound
	}
	f.jobs[j.ID] = j
	return j, nil
}

func (f *fakeReplMgr) Delete(_ context.Context, id uuid.UUID) error {
	if f.delE != nil {
		return f.delE
	}
	if _, ok := f.jobs[id]; !ok {
		return replication.ErrNotFound
	}
	delete(f.jobs, id)
	return nil
}

func (f *fakeReplMgr) Get(_ context.Context, id uuid.UUID) (replication.Job, error) {
	j, ok := f.jobs[id]
	if !ok {
		return replication.Job{}, replication.ErrNotFound
	}
	return j, nil
}

func (f *fakeReplMgr) List(_ context.Context) ([]replication.Job, error) {
	out := make([]replication.Job, 0, len(f.jobs))
	for _, j := range f.jobs {
		out = append(out, j)
	}
	return out, nil
}

func (f *fakeReplMgr) Runs(_ context.Context, id uuid.UUID, limit int) ([]replication.Run, error) {
	runs := f.runs[id]
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

func newTestRouter(h *ReplicationHandler) chi.Router {
	r := chi.NewRouter()
	r.Get("/replication-jobs", h.List)
	r.Post("/replication-jobs", h.Create)
	r.Get("/replication-jobs/{id}", h.Get)
	r.Patch("/replication-jobs/{id}", h.Update)
	r.Delete("/replication-jobs/{id}", h.Delete)
	r.Post("/replication-jobs/{id}/run", h.Run)
	r.Get("/replication-jobs/{id}/runs", h.Runs)
	return r
}

func TestReplicationCreateAndGet(t *testing.T) {
	mgr := newFakeReplMgr()
	h := &ReplicationHandler{Logger: slog.Default(), Mgr: mgr}
	router := newTestRouter(h)

	body := `{"name":"nightly","backend":"zfs","direction":"push","source":{"dataset":"tank/data"},"destination":{"dataset":"backup/data","host":"nas2"},"schedule":"0 2 * * *"}`
	req := httptest.NewRequest(http.MethodPost, "/replication-jobs", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rr.Code, rr.Body.String())
	}
	var got jobResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "nightly" || got.Backend != "zfs" || got.Direction != "push" {
		t.Fatalf("unexpected job: %+v", got)
	}

	// GET
	req = httptest.NewRequest(http.MethodGet, "/replication-jobs/"+got.ID, nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status = %d", rr.Code)
	}
}

func TestReplicationCreateBadInput(t *testing.T) {
	mgr := newFakeReplMgr()
	h := &ReplicationHandler{Logger: slog.Default(), Mgr: mgr}
	router := newTestRouter(h)

	body := `{"name":"","backend":"zfs","direction":"push"}`
	req := httptest.NewRequest(http.MethodPost, "/replication-jobs", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestReplicationGetNotFound(t *testing.T) {
	mgr := newFakeReplMgr()
	h := &ReplicationHandler{Logger: slog.Default(), Mgr: mgr}
	router := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/replication-jobs/"+uuid.New().String(), nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestReplicationListAndDelete(t *testing.T) {
	mgr := newFakeReplMgr()
	id := uuid.New()
	mgr.jobs[id] = replication.Job{ID: id, Name: "j1", Backend: replication.BackendZFS, Direction: replication.DirectionPush, Enabled: true}
	h := &ReplicationHandler{Logger: slog.Default(), Mgr: mgr}
	router := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/replication-jobs", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d", rr.Code)
	}
	var got []jobResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}

	req = httptest.NewRequest(http.MethodDelete, "/replication-jobs/"+id.String(), nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rr.Code)
	}
	if _, ok := mgr.jobs[id]; ok {
		t.Fatalf("job not deleted")
	}
}

func TestReplicationRunDispatches(t *testing.T) {
	mgr := newFakeReplMgr()
	id := uuid.New()
	mgr.jobs[id] = replication.Job{ID: id, Name: "j", Backend: replication.BackendZFS, Direction: replication.DirectionPush, Enabled: true}
	disp := &fakeDispatcher{out: uuid.New()}
	h := &ReplicationHandler{Logger: slog.Default(), Mgr: mgr, Dispatcher: disp}
	router := newTestRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/replication-jobs/"+id.String()+"/run", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted && rr.Code != http.StatusOK {
		t.Fatalf("run status = %d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindReplicationRun {
		t.Fatalf("kind = %q", disp.calls[0].Kind)
	}
	if disp.calls[0].UniqueKey != "replication:job:"+id.String() {
		t.Fatalf("unique key = %q", disp.calls[0].UniqueKey)
	}
}

func TestReplicationRunDuplicateConflict(t *testing.T) {
	mgr := newFakeReplMgr()
	id := uuid.New()
	mgr.jobs[id] = replication.Job{ID: id, Name: "j", Backend: replication.BackendZFS, Direction: replication.DirectionPush, Enabled: true}
	disp := &fakeDispatcher{err: jobs.ErrDuplicate}
	h := &ReplicationHandler{Logger: slog.Default(), Mgr: mgr, Dispatcher: disp}
	router := newTestRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/replication-jobs/"+id.String()+"/run", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rr.Code)
	}
}

func TestReplicationUpdate(t *testing.T) {
	mgr := newFakeReplMgr()
	id := uuid.New()
	mgr.jobs[id] = replication.Job{
		ID:          id,
		Name:        "old",
		Backend:     replication.BackendZFS,
		Direction:   replication.DirectionPush,
		Source:      replication.Source{Dataset: "tank/x"},
		Destination: replication.Destination{Dataset: "backup/x", Host: "nas2"},
		Enabled:     true,
	}
	h := &ReplicationHandler{Logger: slog.Default(), Mgr: mgr}
	router := newTestRouter(h)

	body := `{"schedule":"*/5 * * * *"}`
	req := httptest.NewRequest(http.MethodPatch, "/replication-jobs/"+id.String(), bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch status = %d body=%s", rr.Code, rr.Body.String())
	}
	if mgr.jobs[id].Schedule != "*/5 * * * *" {
		t.Fatalf("schedule not updated: %q", mgr.jobs[id].Schedule)
	}
	if mgr.jobs[id].Source.Dataset != "tank/x" {
		t.Fatalf("source.dataset preserved expected, got %q", mgr.jobs[id].Source.Dataset)
	}
}

func TestReplicationManagerErrPropagation(t *testing.T) {
	mgr := newFakeReplMgr()
	mgr.createE = errors.New("name conflict")
	h := &ReplicationHandler{Logger: slog.Default(), Mgr: mgr}
	router := newTestRouter(h)
	body := `{"name":"x","backend":"zfs","direction":"push","source":{"dataset":"tank/d"},"destination":{"dataset":"backup/d","host":"nas2"}}`
	req := httptest.NewRequest(http.MethodPost, "/replication-jobs", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
