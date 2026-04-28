package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

type fakePoolMgr struct {
	list    []pool.Pool
	listErr error
	detail  *pool.Detail
	getErr  error
}

func (f *fakePoolMgr) List(_ context.Context) ([]pool.Pool, error) {
	return f.list, f.listErr
}
func (f *fakePoolMgr) Get(_ context.Context, _ string) (*pool.Detail, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.detail, nil
}
func (f *fakePoolMgr) Importable(_ context.Context) ([]pool.ImportablePool, error) {
	return nil, nil
}

func TestPoolsList(t *testing.T) {
	h := &PoolsHandler{Logger: newDiscardLogger(), Pools: &fakePoolMgr{
		list: []pool.Pool{{Name: "tank", Health: "ONLINE"}},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []pool.Pool
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 1 || got[0].Name != "tank" {
		t.Errorf("body=%+v", got)
	}
}

func TestPoolsList_EmptyReturnsArrayNotNull(t *testing.T) {
	h := &PoolsHandler{Logger: newDiscardLogger(), Pools: &fakePoolMgr{list: nil}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if rr.Body.String() != "[]\n" {
		t.Errorf("want [] got %q", rr.Body.String())
	}
}

func TestPoolsGet_NotFound(t *testing.T) {
	h := &PoolsHandler{Logger: newDiscardLogger(), Pools: &fakePoolMgr{getErr: pool.ErrNotFound}}
	r := chi.NewRouter()
	r.Get("/api/v1/pools/{name}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools/nope", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPoolsGet_Found(t *testing.T) {
	h := &PoolsHandler{Logger: newDiscardLogger(), Pools: &fakePoolMgr{
		detail: &pool.Detail{Pool: pool.Pool{Name: "tank"}},
	}}
	r := chi.NewRouter()
	r.Get("/api/v1/pools/{name}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools/tank", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status=%d", rr.Code)
	}
	var got pool.Detail
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Pool.Name != "tank" {
		t.Errorf("body=%+v", got)
	}
}

func TestPoolsList_HostErrorReturns500(t *testing.T) {
	h := &PoolsHandler{Logger: newDiscardLogger(), Pools: &fakePoolMgr{listErr: errors.New("boom")}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

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

func TestPoolsGet_HostErrorReturns500(t *testing.T) {
	h := &PoolsHandler{Logger: newDiscardLogger(), Pools: &fakePoolMgr{getErr: errors.New("boom")}}
	r := chi.NewRouter()
	r.Get("/api/v1/pools/{name}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools/tank", nil)
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
