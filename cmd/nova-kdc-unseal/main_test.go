package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFile0600_RoundTripsAndSetsMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "stash")
	want := []byte("hello-master-key")
	if err := writeFile0600(p, want); err != nil {
		t.Fatalf("writeFile0600: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("contents mismatch: got %q want %q", got, want)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestWriteFile0600_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "stash")
	if err := os.WriteFile(p, []byte("old-junk-larger-payload"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := writeFile0600(p, []byte("new")); err != nil {
		t.Fatalf("writeFile0600: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("got %q want %q", got, "new")
	}
}

// runUnseal must be idempotent: a non-empty run-stash short-circuits,
// even if the sealed blob is missing/garbage.
func TestRunUnseal_RunStashAlreadyPresentIsNoOp(t *testing.T) {
	dir := t.TempDir()
	stash := filepath.Join(dir, ".k5.TEST.LOCAL")
	if err := os.WriteFile(stash, []byte("already-here"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := runUnseal(logger, filepath.Join(dir, "no-such-blob.enc"), stash); err != nil {
		t.Fatalf("runUnseal: %v", err)
	}
	got, _ := os.ReadFile(stash)
	if string(got) != "already-here" {
		t.Errorf("run-stash was modified: %q", got)
	}
}

// When neither the sealed blob nor the run-stash exists, runUnseal
// exits cleanly so the KDC can fall back to a non-TPM stash file.
func TestRunUnseal_BlobMissingExitsClean(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	err := runUnseal(logger,
		filepath.Join(dir, "missing.enc"),
		filepath.Join(dir, ".k5.NONE"))
	if err != nil {
		t.Fatalf("expected nil error for missing blob, got %v", err)
	}
}
