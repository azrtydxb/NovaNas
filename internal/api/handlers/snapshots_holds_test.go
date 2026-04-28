package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	"github.com/novanas/nova-nas/internal/jobs"
)

func TestSnapshotsHold_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SnapshotsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := routedHandler(http.MethodPost, "/api/v1/snapshots/{fullname}/hold", h.Hold)
	target := "/api/v1/snapshots/" + url.PathEscape("tank/home@snap1") + "/hold"
	body := `{"tag":"keepme","recursive":true}`
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindSnapshotHold {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.SnapshotHoldPayload)
	if !ok || p.Snapshot != "tank/home@snap1" || p.Tag != "keepme" || !p.Recursive {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

func TestSnapshotsHold_BadName400(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &SnapshotsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := routedHandler(http.MethodPost, "/api/v1/snapshots/{fullname}/hold", h.Hold)
	// dataset (no '@') fails snapshot validation
	target := "/api/v1/snapshots/" + url.PathEscape("tank/home") + "/hold"
	body := `{"tag":"keepme"}`
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	if len(disp.calls) != 0 {
		t.Errorf("should not dispatch")
	}
}

func TestSnapshotsHold_EmptyTag400(t *testing.T) {
	disp := &fakeDispatcher{}
	h := &SnapshotsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := routedHandler(http.MethodPost, "/api/v1/snapshots/{fullname}/hold", h.Hold)
	target := "/api/v1/snapshots/" + url.PathEscape("tank/home@snap1") + "/hold"
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(`{"tag":""}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestSnapshotsRelease_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SnapshotsWriteHandler{Logger: newDiscardLogger(), Dispatcher: disp}

	r := routedHandler(http.MethodPost, "/api/v1/snapshots/{fullname}/release", h.Release)
	target := "/api/v1/snapshots/" + url.PathEscape("tank/home@snap1") + "/release"
	body := `{"tag":"keepme"}`
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindSnapshotRelease {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestSnapshotsHolds_Sync(t *testing.T) {
	mgr := &snapshot.Manager{
		ZFSBin: "zfs",
		Runner: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			// holds -H -p tank/home@snap1
			if len(args) != 4 || args[0] != "holds" {
				t.Errorf("args=%v", args)
			}
			return []byte("tank/home@snap1\tkeepme\t1700000000\n"), nil
		},
	}
	h := &SnapshotsHoldsHandler{Logger: newDiscardLogger(), Snapshot: mgr}

	r := routedHandler(http.MethodGet, "/api/v1/snapshots/{fullname}/holds", h.Holds)
	target := "/api/v1/snapshots/" + url.PathEscape("tank/home@snap1") + "/holds"
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var holds []snapshot.Hold
	if err := json.NewDecoder(rr.Body).Decode(&holds); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(holds) != 1 || holds[0].Tag != "keepme" || holds[0].CreationUnix != 1700000000 {
		t.Errorf("holds=%+v", holds)
	}
}

func TestSnapshotsHolds_BadName400(t *testing.T) {
	mgr := &snapshot.Manager{ZFSBin: "zfs"}
	h := &SnapshotsHoldsHandler{Logger: newDiscardLogger(), Snapshot: mgr}

	r := routedHandler(http.MethodGet, "/api/v1/snapshots/{fullname}/holds", h.Holds)
	target := "/api/v1/snapshots/" + url.PathEscape("tank/home") + "/holds"
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}
