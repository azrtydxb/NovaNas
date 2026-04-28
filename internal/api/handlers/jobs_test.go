package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type fakeJobQ struct {
	get    storedb.Job
	getErr error
	list   []storedb.Job
}

func (f *fakeJobQ) GetJob(_ context.Context, _ pgtype.UUID) (storedb.Job, error) {
	return f.get, f.getErr
}
func (f *fakeJobQ) ListJobs(_ context.Context, _ storedb.ListJobsParams) ([]storedb.Job, error) {
	return f.list, nil
}
func (f *fakeJobQ) CancelJob(_ context.Context, _ pgtype.UUID) error { return nil }

func TestJobsGet_404(t *testing.T) {
	h := &JobsHandler{Logger: newDiscardLogger(), Q: &fakeJobQ{getErr: storedb.ErrNoRows}}
	r := chi.NewRouter()
	r.Get("/jobs/{id}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+uuid.New().String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestJobsGet_BadID(t *testing.T) {
	h := &JobsHandler{Logger: newDiscardLogger(), Q: &fakeJobQ{}}
	r := chi.NewRouter()
	r.Get("/jobs/{id}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/jobs/not-a-uuid", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestJobsList_OK(t *testing.T) {
	h := &JobsHandler{Logger: newDiscardLogger(), Q: &fakeJobQ{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestJobsCancel_204(t *testing.T) {
	h := &JobsHandler{Logger: newDiscardLogger(), Q: &fakeJobQ{}}
	r := chi.NewRouter()
	r.Delete("/jobs/{id}", h.Cancel)
	req := httptest.NewRequest(http.MethodDelete, "/jobs/"+uuid.New().String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("status=%d", rr.Code)
	}
}
