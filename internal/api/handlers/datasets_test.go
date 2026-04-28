package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
)

type fakeDatasetMgr struct {
	list      []dataset.Dataset
	listErr   error
	lastRoot  string
	detail    *dataset.Detail
	getErr    error
}

func (f *fakeDatasetMgr) List(_ context.Context, root string) ([]dataset.Dataset, error) {
	f.lastRoot = root
	return f.list, f.listErr
}
func (f *fakeDatasetMgr) Get(_ context.Context, _ string) (*dataset.Detail, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.detail, nil
}

func TestDatasetsList(t *testing.T) {
	h := &DatasetsHandler{Logger: newDiscardLogger(), Datasets: &fakeDatasetMgr{
		list: []dataset.Dataset{{Name: "tank/home", Type: "filesystem"}},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []dataset.Dataset
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 1 || got[0].Name != "tank/home" {
		t.Errorf("body=%+v", got)
	}
}

func TestDatasetsList_Empty(t *testing.T) {
	h := &DatasetsHandler{Logger: newDiscardLogger(), Datasets: &fakeDatasetMgr{list: nil}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Body.String() != "[]\n" {
		t.Errorf("want [] got %q", rr.Body.String())
	}
}

func TestDatasetsList_HostErrorReturns500(t *testing.T) {
	h := &DatasetsHandler{Logger: newDiscardLogger(), Datasets: &fakeDatasetMgr{listErr: errors.New("boom")}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status=%d", rr.Code)
	}
	if rr.Body.String() == "" || rr.Body.String() == "boom\n" {
		t.Errorf("expected envelope, got %q", rr.Body.String())
	}
}

func TestDatasetsGet_URLEncoded(t *testing.T) {
	h := &DatasetsHandler{Logger: newDiscardLogger(), Datasets: &fakeDatasetMgr{
		detail: &dataset.Detail{Dataset: dataset.Dataset{Name: "tank/home"}},
	}}
	r := chi.NewRouter()
	r.Get("/api/v1/datasets/{fullname}", h.Get)

	target := "/api/v1/datasets/" + url.PathEscape("tank/home")
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDatasetsGet_NotFound(t *testing.T) {
	h := &DatasetsHandler{Logger: newDiscardLogger(), Datasets: &fakeDatasetMgr{getErr: dataset.ErrNotFound}}
	r := chi.NewRouter()
	r.Get("/api/v1/datasets/{fullname}", h.Get)
	target := "/api/v1/datasets/" + url.PathEscape("tank/missing")
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestDatasetsList_ForwardsPoolQueryAsRoot(t *testing.T) {
	mgr := &fakeDatasetMgr{}
	h := &DatasetsHandler{Logger: newDiscardLogger(), Datasets: mgr}

	// no query → root should be empty
	rr1 := httptest.NewRecorder()
	h.List(rr1, httptest.NewRequest(http.MethodGet, "/api/v1/datasets", nil))
	if mgr.lastRoot != "" {
		t.Errorf("no-query lastRoot=%q want empty", mgr.lastRoot)
	}

	// ?pool=tank/home → root should be "tank/home"
	rr2 := httptest.NewRecorder()
	h.List(rr2, httptest.NewRequest(http.MethodGet, "/api/v1/datasets?pool=tank/home", nil))
	if mgr.lastRoot != "tank/home" {
		t.Errorf("?pool=tank/home lastRoot=%q", mgr.lastRoot)
	}
}

func TestDatasetsGet_HostErrorReturns500(t *testing.T) {
	h := &DatasetsHandler{Logger: newDiscardLogger(), Datasets: &fakeDatasetMgr{getErr: errors.New("boom")}}
	r := chi.NewRouter()
	r.Get("/api/v1/datasets/{fullname}", h.Get)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home")
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Error != "host_error" {
		t.Errorf("error=%q", env.Error)
	}
	if env.Message == "boom" {
		t.Errorf("internal err leaked: %q", env.Message)
	}
}
