package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/novanas/nova-nas/internal/host/disks"
)

type fakeDiskLister struct {
	result []disks.Disk
	err    error
}

func (f *fakeDiskLister) List(_ context.Context) ([]disks.Disk, error) {
	return f.result, f.err
}

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestDisksList(t *testing.T) {
	h := &DisksHandler{
		Logger: newDiscardLogger(),
		Lister: &fakeDiskLister{result: []disks.Disk{
			{Name: "sda", SizeBytes: 1000, Rotational: true},
		}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type=%q", ct)
	}
	var got []disks.Disk
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "sda" {
		t.Errorf("body=%+v", got)
	}
}

func TestDisksList_EmptyReturnsArrayNotNull(t *testing.T) {
	h := &DisksHandler{
		Logger: newDiscardLogger(),
		Lister: &fakeDiskLister{result: nil},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	body := rr.Body.String()
	if body != "[]\n" {
		t.Errorf("want [] body, got %q", body)
	}
}

func TestDisksList_HostErrorReturns500(t *testing.T) {
	h := &DisksHandler{
		Logger: newDiscardLogger(),
		Lister: &fakeDiskLister{err: errors.New("boom")},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks", nil)
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
	// Detail of underlying err must not leak
	if env.Message == "boom" {
		t.Errorf("internal error leaked into response: %q", env.Message)
	}
}
