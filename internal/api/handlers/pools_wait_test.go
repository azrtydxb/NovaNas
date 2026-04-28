package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

func TestPoolsWait_Success(t *testing.T) {
	called := false
	mgr := &pool.Manager{
		ZpoolBin: "zpool",
		Runner: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			called = true
			// args should be ["wait", "-t", <activity>, <name>]
			if len(args) != 4 || args[0] != "wait" || args[1] != "-t" || args[2] != "scrub" || args[3] != "tank" {
				t.Errorf("args=%v", args)
			}
			return nil, nil
		},
	}
	h := &PoolsWaitHandler{Logger: newDiscardLogger(), Pools: mgr}

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/wait", h.Wait)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/wait?activity=scrub", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Errorf("manager not invoked")
	}
}

func TestPoolsWait_MissingActivity400(t *testing.T) {
	h := &PoolsWaitHandler{Logger: newDiscardLogger(), Pools: &pool.Manager{}}
	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/wait", h.Wait)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/wait", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestPoolsWait_BadActivity500(t *testing.T) {
	// Manager.Wait validates activity itself; an invalid one gives a
	// non-host error, which the handler maps to 500.
	mgr := &pool.Manager{
		ZpoolBin: "zpool",
		Runner: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			t.Fatal("runner should not be called for bad activity")
			return nil, nil
		},
	}
	h := &PoolsWaitHandler{Logger: newDiscardLogger(), Pools: mgr}
	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/wait", h.Wait)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/wait?activity=garbage", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestPoolsWait_BadName400(t *testing.T) {
	h := &PoolsWaitHandler{Logger: newDiscardLogger(), Pools: &pool.Manager{}}
	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/wait", h.Wait)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/123bad/wait?activity=scrub", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}
