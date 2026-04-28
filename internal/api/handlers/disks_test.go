package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/novanas/nova-nas/internal/host/disks"
)

type fakeDiskLister struct{ result []disks.Disk }

func (f *fakeDiskLister) List(_ context.Context) ([]disks.Disk, error) {
	return f.result, nil
}

func TestDisksList(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := &DisksHandler{
		Logger: logger,
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
	var got []disks.Disk
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "sda" {
		t.Errorf("body=%+v", got)
	}
}
