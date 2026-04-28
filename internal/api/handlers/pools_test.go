package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

type fakePoolMgr struct {
	list   []pool.Pool
	detail *pool.Detail
	getErr error
}

func (f *fakePoolMgr) List(_ context.Context) ([]pool.Pool, error) { return f.list, nil }
func (f *fakePoolMgr) Get(_ context.Context, _ string) (*pool.Detail, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.detail, nil
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
}
