package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// fakeLoader satisfies keyLoader. Each entry in load tracks whether
// LoadKey was called for that dataset; values control the returned
// error (nil = success).
type fakeLoader struct {
	listed []string
	listErr error
	load    map[string]error
	called  []string
}

func (f *fakeLoader) ListEscrowedDatasets(_ context.Context) ([]string, error) {
	return f.listed, f.listErr
}

func (f *fakeLoader) LoadKey(_ context.Context, full string) error {
	f.called = append(f.called, full)
	if e, ok := f.load[full]; ok {
		return e
	}
	return nil
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLoadAll_NoDatasets(t *testing.T) {
	f := &fakeLoader{}
	if err := loadAll(context.Background(), newTestLogger(), f, time.Second); err != nil {
		t.Errorf("unexpected err: %v", err)
	}
	if len(f.called) != 0 {
		t.Errorf("LoadKey called %d times", len(f.called))
	}
}

func TestLoadAll_AllSucceed(t *testing.T) {
	f := &fakeLoader{
		listed: []string{"tank/a", "tank/b"},
	}
	if err := loadAll(context.Background(), newTestLogger(), f, time.Second); err != nil {
		t.Errorf("unexpected err: %v", err)
	}
	if len(f.called) != 2 {
		t.Errorf("called=%v", f.called)
	}
}

func TestLoadAll_PartialFailure_FailsClosed(t *testing.T) {
	// Per spec: "Failure to unwrap (PCR mismatch) MUST fail closed."
	// Even partial failure produces non-zero exit.
	f := &fakeLoader{
		listed: []string{"tank/a", "tank/b"},
		load:   map[string]error{"tank/b": errors.New("pcr mismatch")},
	}
	err := loadAll(context.Background(), newTestLogger(), f, time.Second)
	if err == nil {
		t.Fatal("expected non-nil error on partial failure")
	}
	if !strings.Contains(err.Error(), "1/2") {
		t.Errorf("expected count summary, got %v", err)
	}
	// Iteration continues even after one fails.
	if len(f.called) != 2 {
		t.Errorf("expected both attempted, got %v", f.called)
	}
}

func TestLoadAll_ListError(t *testing.T) {
	f := &fakeLoader{listErr: errors.New("backend down")}
	if err := loadAll(context.Background(), newTestLogger(), f, time.Second); err == nil {
		t.Fatal("expected error from list")
	}
}

// Smoke test: newLogger returns a non-nil logger for any input.
func TestNewLogger(t *testing.T) {
	for _, l := range []string{"debug", "info", "warn", "error", "garbage"} {
		if newLogger(l) == nil {
			t.Errorf("nil logger for %q", l)
		}
	}
	// Sanity-check that the logger writes something.
	var buf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&buf, nil))
	lg.Info("hi")
	if buf.Len() == 0 {
		t.Error("logger produced no output")
	}
}
