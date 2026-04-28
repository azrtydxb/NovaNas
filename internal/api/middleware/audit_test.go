package middleware

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type fakeAuditQ struct {
	called int
	got    storedb.InsertAuditParams
}

func (f *fakeAuditQ) InsertAudit(_ context.Context, p storedb.InsertAuditParams) error {
	f.called++
	f.got = p
	return nil
}

func TestAudit_RecordsAccepted(t *testing.T) {
	fq := &fakeAuditQ{}
	mw := Audit(fq)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools", bytes.NewBufferString(`{"name":"tank"}`))
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, req)

	if !called {
		t.Fatal("next not called")
	}
	if fq.called != 1 {
		t.Fatalf("audit insert calls=%d", fq.called)
	}
	if fq.got.Result != "accepted" {
		t.Errorf("result=%q", fq.got.Result)
	}
	if fq.got.Action != "POST /api/v1/pools" {
		t.Errorf("action=%q", fq.got.Action)
	}
	if fq.got.Actor != nil {
		t.Errorf("actor should be nil in v1, got %v", *fq.got.Actor)
	}
	if string(fq.got.Payload) != `{"name":"tank"}` {
		t.Errorf("payload=%q", fq.got.Payload)
	}
}

func TestAudit_SkipsGET(t *testing.T) {
	fq := &fakeAuditQ{}
	mw := Audit(fq)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools", nil)
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, req)
	if fq.called != 0 {
		t.Errorf("GET should not audit; called=%d", fq.called)
	}
}

func TestAudit_RejectedOn4xx(t *testing.T) {
	fq := &fakeAuditQ{}
	mw := Audit(fq)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools", bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, req)
	if fq.called != 1 {
		t.Fatalf("audit calls=%d", fq.called)
	}
	if fq.got.Result != "rejected" {
		t.Errorf("result=%q", fq.got.Result)
	}
}
