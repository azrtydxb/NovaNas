package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
)

// TestDatasetsSend_Stream verifies that a successful Send copies bytes
// from the manager's StreamRunner straight to the HTTP body.
func TestDatasetsSend_Stream(t *testing.T) {
	mgr := &dataset.Manager{
		ZFSBin: "zfs",
		StreamRunner: func(_ context.Context, _ string, _ io.Reader, w io.Writer, _ ...string) error {
			_, err := w.Write([]byte("STREAMDATA"))
			return err
		},
	}
	h := &DatasetsStreamHandler{Logger: newDiscardLogger(), Dataset: mgr}

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/send", h.Send)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home@snap1") + "/send?recursive=true&compressed=true"
	req := httptest.NewRequest(http.MethodPost, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("Content-Type=%q", rr.Header().Get("Content-Type"))
	}
	if rr.Body.String() != "STREAMDATA" {
		t.Errorf("body=%q", rr.Body.String())
	}
}

func TestDatasetsSend_BadName400(t *testing.T) {
	h := &DatasetsStreamHandler{Logger: newDiscardLogger(), Dataset: &dataset.Manager{}}
	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/send", h.Send)
	// not a snapshot (no @)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home") + "/send"
	req := httptest.NewRequest(http.MethodPost, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

// TestDatasetsReceive_Stream verifies the request body is forwarded to
// the manager's StreamRunner.
func TestDatasetsReceive_Stream(t *testing.T) {
	var got string
	mgr := &dataset.Manager{
		ZFSBin: "zfs",
		StreamRunner: func(_ context.Context, _ string, stdin io.Reader, _ io.Writer, _ ...string) error {
			b, err := io.ReadAll(stdin)
			if err != nil {
				return err
			}
			got = string(b)
			return nil
		},
	}
	h := &DatasetsStreamHandler{Logger: newDiscardLogger(), Dataset: mgr}

	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/receive", h.Receive)
	target := "/api/v1/datasets/" + url.PathEscape("tank/restored") + "/receive?force=true"
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader("INCOMING"))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got != "INCOMING" {
		t.Errorf("manager saw %q", got)
	}
}

func TestDatasetsReceive_HostError500(t *testing.T) {
	mgr := &dataset.Manager{
		ZFSBin: "zfs",
		StreamRunner: func(_ context.Context, _ string, _ io.Reader, _ io.Writer, _ ...string) error {
			return errors.New("zfs receive failed")
		},
	}
	h := &DatasetsStreamHandler{Logger: newDiscardLogger(), Dataset: mgr}
	r := routedHandler(http.MethodPost, "/api/v1/datasets/{fullname}/receive", h.Receive)
	target := "/api/v1/datasets/" + url.PathEscape("tank/restored") + "/receive"
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader("data"))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status=%d", rr.Code)
	}
}
