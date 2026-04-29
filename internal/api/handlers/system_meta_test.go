package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSystemMetaVersionReturnsGoVersion(t *testing.T) {
	h := &SystemMetaHandler{BuildCommit: "abc1234", BuildTime: "2026-01-01T00:00:00Z"}
	rr := httptest.NewRecorder()
	h.GetVersion(rr, httptest.NewRequest("GET", "/system/version", nil))
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	var v SystemVersion
	if err := json.NewDecoder(rr.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(v.GoVersion, "go") {
		t.Errorf("goVersion=%s", v.GoVersion)
	}
	if v.Commit != "abc1234" {
		t.Errorf("commit=%s", v.Commit)
	}
}

func TestSystemMetaUpdatesStub(t *testing.T) {
	h := &SystemMetaHandler{}
	rr := httptest.NewRecorder()
	h.GetUpdates(rr, httptest.NewRequest("GET", "/system/updates", nil))
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	var u SystemUpdate
	if err := json.NewDecoder(rr.Body).Decode(&u); err != nil {
		t.Fatal(err)
	}
	if u.Available {
		t.Errorf("expected available=false, got %+v", u)
	}
	if u.Status != "idle" {
		t.Errorf("status=%s", u.Status)
	}
}
