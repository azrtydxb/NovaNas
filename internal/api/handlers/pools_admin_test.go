package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/jobs"
)

func TestPoolsCheckpoint_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/checkpoint", h.Checkpoint)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/checkpoint", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindPoolCheckpoint {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p := disp.calls[0].Payload.(jobs.PoolCheckpointPayload)
	if p.Name != "tank" {
		t.Errorf("payload=%+v", p)
	}
}

func TestPoolsCheckpoint_BadName400(t *testing.T) {
	disp := &fakeDispatcher{}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/checkpoint", h.Checkpoint)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/bad@name/checkpoint", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch")
	}
}

func TestPoolsDiscardCheckpoint_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/discard-checkpoint", h.DiscardCheckpoint)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/discard-checkpoint", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	if disp.calls[0].Kind != jobs.KindPoolDiscardCheckpoint {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestPoolsUpgrade_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/upgrade", h.Upgrade)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/upgrade", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	p := disp.calls[0].Payload.(jobs.PoolUpgradePayload)
	if p.Name != "tank" || p.All {
		t.Errorf("payload=%+v", p)
	}
}

func TestPoolsUpgrade_All(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/upgrade", h.Upgrade)
	// pool name doesn't matter when all=true; using a placeholder.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/_/upgrade?all=true", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	p := disp.calls[0].Payload.(jobs.PoolUpgradePayload)
	if !p.All {
		t.Errorf("expected All=true, got %+v", p)
	}
}

func TestPoolsUpgrade_BadName400(t *testing.T) {
	disp := &fakeDispatcher{}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/upgrade", h.Upgrade)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/bad@name/upgrade", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestPoolsReguid_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/pools/{name}/reguid", h.Reguid)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/tank/reguid", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	if disp.calls[0].Kind != jobs.KindPoolReguid {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestPoolsSync_AllPools(t *testing.T) {
	called := false
	mgr := &pool.Manager{
		ZpoolBin: "zpool",
		Runner: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			called = true
			if len(args) != 1 || args[0] != "sync" {
				t.Errorf("args=%v want [sync]", args)
			}
			return nil, nil
		},
	}
	h := &PoolsSyncHandler{Logger: newDiscardLogger(), Pool: mgr}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/sync", nil)
	rr := httptest.NewRecorder()
	h.Sync(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Errorf("runner not called")
	}
}

func TestPoolsSync_NamedPools(t *testing.T) {
	mgr := &pool.Manager{
		ZpoolBin: "zpool",
		Runner: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(args) != 3 || args[0] != "sync" || args[1] != "tank" || args[2] != "tank2" {
				t.Errorf("args=%v", args)
			}
			return nil, nil
		},
	}
	h := &PoolsSyncHandler{Logger: newDiscardLogger(), Pool: mgr}

	body := `{"names":["tank","tank2"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/sync", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Sync(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPoolsSync_BadName400(t *testing.T) {
	mgr := &pool.Manager{ZpoolBin: "zpool"}
	h := &PoolsSyncHandler{Logger: newDiscardLogger(), Pool: mgr}

	body := `{"names":["bad@name"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools/sync", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Sync(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}
