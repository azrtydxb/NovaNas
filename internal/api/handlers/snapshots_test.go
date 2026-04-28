package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
)

type fakeSnapMgr struct {
	list     []snapshot.Snapshot
	listErr  error
	lastRoot string
}

func (f *fakeSnapMgr) List(_ context.Context, root string) ([]snapshot.Snapshot, error) {
	f.lastRoot = root
	return f.list, f.listErr
}

func TestSnapshotsList(t *testing.T) {
	h := &SnapshotsHandler{Logger: newDiscardLogger(), Snapshots: &fakeSnapMgr{
		list: []snapshot.Snapshot{{Name: "tank/home@a", Dataset: "tank/home", ShortName: "a"}},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []snapshot.Snapshot
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 1 || got[0].Name != "tank/home@a" {
		t.Errorf("body=%+v", got)
	}
}

func TestSnapshotsList_Empty(t *testing.T) {
	h := &SnapshotsHandler{Logger: newDiscardLogger(), Snapshots: &fakeSnapMgr{list: nil}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Body.String() != "[]\n" {
		t.Errorf("want [] got %q", rr.Body.String())
	}
}

func TestSnapshotsList_HostErrorReturns500(t *testing.T) {
	h := &SnapshotsHandler{Logger: newDiscardLogger(), Snapshots: &fakeSnapMgr{listErr: errors.New("boom")}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status=%d", rr.Code)
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

func TestSnapshotsList_ForwardsDatasetQueryAsRoot(t *testing.T) {
	mgr := &fakeSnapMgr{}
	h := &SnapshotsHandler{Logger: newDiscardLogger(), Snapshots: mgr}

	rr := httptest.NewRecorder()
	h.List(rr, httptest.NewRequest(http.MethodGet, "/api/v1/snapshots?dataset=tank/home", nil))
	if mgr.lastRoot != "tank/home" {
		t.Errorf("lastRoot=%q", mgr.lastRoot)
	}
}
