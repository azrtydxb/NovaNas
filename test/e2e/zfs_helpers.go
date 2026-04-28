//go:build e2e

// Package e2e contains end-to-end tests that exercise real ZFS commands
// against sparse loopback devices. Build-tagged `e2e` so they only run
// on hosts where ZFS is installed and the runner has root (or sufficient
// capabilities for losetup/zpool/zfs).
package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// makeLoopback creates a sparse file of the given size and attaches it
// as a loop device. Returns the loop device path (e.g. /dev/loop10).
// Caller cleanup is registered via t.Cleanup so the loop is detached at
// test end.
func makeLoopback(t *testing.T, sizeBytes int64) string {
	t.Helper()
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "disk.img")
	if err := os.WriteFile(imgPath, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(imgPath, sizeBytes); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("losetup", "-f", "--show", imgPath).CombinedOutput()
	if err != nil {
		t.Fatalf("losetup: %v\n%s", err, out)
	}
	loopDev := strings.TrimSpace(string(out))
	t.Cleanup(func() { _ = exec.Command("losetup", "-d", loopDev).Run() })
	return loopDev
}

// destroyPoolIfExists is a best-effort cleanup helper — silently
// swallows errors so it can be called from t.Cleanup unconditionally.
func destroyPoolIfExists(name string) {
	_ = exec.Command("zpool", "destroy", "-f", name).Run()
}

// uniquePoolName returns a name unique per test (pid + sanitized test
// name). ZFS pool names are constrained to a small alphabet, so any
// '/' or other illegal char from the test path is replaced with '_'.
// Registers a t.Cleanup that destroys the pool if the test forgets to.
func uniquePoolName(t *testing.T) string {
	t.Helper()
	safe := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	name := fmt.Sprintf("e2e_%d_%s", os.Getpid(), safe)
	if len(name) > 31 {
		name = name[:31]
	}
	t.Cleanup(func() { destroyPoolIfExists(name) })
	return name
}

// run is a small shell-out helper used by tests when they need to
// execute ad-hoc tooling beyond what the Manager packages expose.
func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
