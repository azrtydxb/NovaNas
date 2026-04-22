// Package install runs the post-selection install pipeline.
package install

import (
	"fmt"
	"os"
	"os/exec"
)

// RAUCExtractor unpacks a RAUC bundle's rootfs onto a mounted slot.
//
// RAUC bundles are signed squashfs archives. For the scaffold we use
// `unsquashfs` directly and skip signature verification; the real signing
// infrastructure lands with wave 6+.
//
// TODO(wave-6): verify bundle signature against the embedded public key.
type RAUCExtractor struct {
	DryRun bool
	Log    func(format string, args ...any)
	Exec   func(name string, args ...string) error
}

// Verify performs best-effort checks. Currently only existence/size.
func (r *RAUCExtractor) Verify(bundlePath string) error {
	st, err := os.Stat(bundlePath)
	if err != nil {
		return fmt.Errorf("bundle not found: %w", err)
	}
	if st.Size() < 1024*1024 {
		return fmt.Errorf("bundle suspiciously small (%d bytes)", st.Size())
	}
	if r.Log != nil {
		r.Log("rauc bundle %s size=%d bytes (signature verify TODO)", bundlePath, st.Size())
	}
	return nil
}

// Extract unpacks the bundle's rootfs onto the target mount point.
func (r *RAUCExtractor) Extract(bundlePath, mountpoint string) error {
	if r.Log != nil {
		r.Log("extracting %s -> %s", bundlePath, mountpoint)
	}
	if r.DryRun {
		return nil
	}
	exe := r.Exec
	if exe == nil {
		exe = func(name string, args ...string) error {
			cmd := exec.Command(name, args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("%s: %w: %s", name, err, string(out))
			}
			return nil
		}
	}
	// unsquashfs -f -d <mountpoint> <bundle>
	return exe("unsquashfs", "-f", "-d", mountpoint, bundlePath)
}
