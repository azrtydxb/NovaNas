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

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/jobs"
)

func TestDatasetsBookmark_Returns202(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newDatasetsLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/bookmark", h.Bookmark)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home@snap1") + "/bookmark"
	body := `{"bookmark":"tank/home#b1"}`
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindDatasetBookmark {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
	p, ok := disp.calls[0].Payload.(jobs.DatasetBookmarkPayload)
	if !ok || p.Snapshot != "tank/home@snap1" || p.Bookmark != "tank/home#b1" {
		t.Errorf("payload=%+v", disp.calls[0].Payload)
	}
}

func TestDatasetsBookmark_BadName400(t *testing.T) {
	disp := &fakeDispatcher{}
	h := newDatasetsLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/bookmark", h.Bookmark)
	// URL fullname is a dataset (no '@'), so it fails snapshot validation.
	target := "/api/v1/datasets/" + url.PathEscape("tank/home") + "/bookmark"
	body := `{"bookmark":"tank/home#b1"}`
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

func TestDatasetsDestroyBookmark_ShortName(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newDatasetsLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/destroy-bookmark", h.DestroyBookmark)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home") + "/destroy-bookmark"
	body := `{"bookmark":"b1"}`
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	p := disp.calls[0].Payload.(jobs.DatasetDestroyBookmarkPayload)
	if p.Bookmark != "tank/home#b1" {
		t.Errorf("bookmark=%q want tank/home#b1", p.Bookmark)
	}
}

func TestDatasetsDestroyBookmark_FullName(t *testing.T) {
	disp := &fakeDispatcher{out: uuid.New()}
	h := newDatasetsLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/destroy-bookmark", h.DestroyBookmark)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home") + "/destroy-bookmark"
	body := `{"bookmark":"tank/home#b1"}`
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	p := disp.calls[0].Payload.(jobs.DatasetDestroyBookmarkPayload)
	if p.Bookmark != "tank/home#b1" {
		t.Errorf("bookmark=%q", p.Bookmark)
	}
}

func TestDatasetsDestroyBookmark_PrefixMismatch400(t *testing.T) {
	disp := &fakeDispatcher{}
	h := newDatasetsLifecycleHandler(disp)

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/destroy-bookmark", h.DestroyBookmark)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home") + "/destroy-bookmark"
	body := `{"bookmark":"tank/other#b1"}`
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

func TestDatasetsQuery_Diff(t *testing.T) {
	called := false
	mgr := &dataset.Manager{
		ZFSBin: "zfs",
		Runner: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			called = true
			// args: diff -H tank/home@a tank/home@b
			if len(args) != 4 || args[0] != "diff" || args[1] != "-H" {
				t.Errorf("args=%v", args)
			}
			return []byte("M\t/tank/home/file\n+\t/tank/home/new\n"), nil
		},
	}
	h := &DatasetsQueryHandler{Logger: newDiscardLogger(), Dataset: mgr}

	r := routedHandler(http.MethodGet, "/api/v1/datasets/{fullname}/diff", h.Diff)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home@a") + "/diff?to=tank/home@b"
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Errorf("runner not called")
	}
	var entries []dataset.DatasetDiffEntry
	if err := json.NewDecoder(rr.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 2 || entries[0].Change != "M" || entries[1].Change != "+" {
		t.Errorf("entries=%+v", entries)
	}
}

func TestDatasetsQuery_Diff_BadFrom400(t *testing.T) {
	mgr := &dataset.Manager{ZFSBin: "zfs"}
	h := &DatasetsQueryHandler{Logger: newDiscardLogger(), Dataset: mgr}

	r := routedHandler(http.MethodGet, "/api/v1/datasets/{fullname}/diff", h.Diff)
	// Without '@' it's not a snapshot.
	target := "/api/v1/datasets/" + url.PathEscape("tank/home") + "/diff"
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestDatasetsQuery_ListBookmarks(t *testing.T) {
	mgr := &dataset.Manager{
		ZFSBin: "zfs",
		Runner: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			// list -H -p -t bookmark -o name,creation -r tank/home
			if args[0] != "list" {
				t.Errorf("args=%v", args)
			}
			return []byte("tank/home#b1\t1700000000\n"), nil
		},
	}
	h := &DatasetsQueryHandler{Logger: newDiscardLogger(), Dataset: mgr}

	r := routedHandler(http.MethodGet, "/api/v1/datasets/{fullname}/bookmarks", h.ListBookmarks)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home") + "/bookmarks"
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var bms []dataset.Bookmark
	if err := json.NewDecoder(rr.Body).Decode(&bms); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(bms) != 1 || bms[0].Name != "tank/home#b1" || bms[0].CreationUnix != 1700000000 {
		t.Errorf("bms=%+v", bms)
	}
}

