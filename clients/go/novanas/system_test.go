package novanas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetSystemVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/system/version" {
			t.Errorf("path=%s", r.URL.Path)
		}
		writeJSON(t, w, 200, SystemVersion{GoVersion: "go1.22.0", Commit: "abc"})
	}))
	defer srv.Close()
	got, err := newTestClient(t, srv).GetSystemVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Commit != "abc" {
		t.Errorf("got=%+v", got)
	}
}

func TestGetSystemUpdatesStub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/system/updates" {
			t.Errorf("path=%s", r.URL.Path)
		}
		writeJSON(t, w, 200, SystemUpdate{Available: false, Reason: "stub", Status: "idle"})
	}))
	defer srv.Close()
	got, err := newTestClient(t, srv).GetSystemUpdates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Available {
		t.Errorf("got=%+v", got)
	}
}
