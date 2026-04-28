package middleware

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type fakeAuditQ struct {
	called int
	got    storedb.InsertAuditParams
	err    error
}

func (f *fakeAuditQ) InsertAudit(_ context.Context, p storedb.InsertAuditParams) error {
	f.called++
	f.got = p
	return f.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestAudit_RecordsAccepted(t *testing.T) {
	fq := &fakeAuditQ{}
	mw := Audit(fq, discardLogger())

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
	mw := Audit(fq, discardLogger())
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
	mw := Audit(fq, discardLogger())
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

// Panic in handler must not skip the audit row when Audit is registered
// outside Recoverer (the order enforced in server.go). We model the chain
// here directly: outer = Audit, inner = Recoverer, then the panicking
// handler.
func TestAudit_RecordsOnPanic(t *testing.T) {
	fq := &fakeAuditQ{}
	logger := discardLogger()

	panicHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("kaboom")
	})
	chain := Audit(fq, logger)(Recoverer(logger)(panicHandler))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools", bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d (Recoverer should have written 500)", rr.Code)
	}
	if fq.called != 1 {
		t.Fatalf("expected audit row on panicked request, called=%d", fq.called)
	}
	if fq.got.Result != "rejected" {
		t.Errorf("result=%q (panic should be rejected)", fq.got.Result)
	}
}

// On audit-insert failure, the request must still succeed; the failure
// is logged but not propagated to the client.
func TestAudit_LogsButDoesNotFailOnInsertError(t *testing.T) {
	fq := &fakeAuditQ{err: errors.New("db down")}
	mw := Audit(fq, discardLogger())

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools", bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("client status changed by audit failure: %d", rr.Code)
	}
}
