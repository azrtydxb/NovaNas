// Package install runs the post-selection install pipeline.
package install

import (
	"fmt"
	"os"
	"os/exec"
)

// DefaultRAUCKeyring is the default path to the RAUC signing keyring in
// the live ISO / installed system (shipped by A5-OS in os/rauc/keyring.pem).
const DefaultRAUCKeyring = "/etc/rauc/keyring.pem"

// RAUCExtractor unpacks a RAUC bundle's rootfs onto a mounted slot.
//
// RAUC bundles are signed squashfs archives. Signature verification is
// delegated to the `rauc` binary (which understands both classic CMS
// and verity-format bundles) because re-implementing RAUC's signature
// parser in Go is too invasive for the scaffold. Bundling the `rauc`
// tool in the installer/live ISO is acceptable for v1; see
// os/rauc/keyring.pem for the keyring shipped by A5-OS.
//
// TODO(wave-7): switch to an in-process signature verifier using
// crypto/x509 + PKCS#7 so we can verify without a runtime dependency on
// the rauc binary in constrained environments.
type RAUCExtractor struct {
	DryRun bool
	Log    func(format string, args ...any)
	Exec   func(name string, args ...string) error

	// KeyringPath overrides the default RAUC signing keyring. Empty
	// uses DefaultRAUCKeyring.
	KeyringPath string
	// RAUCBinary overrides the path to the `rauc` binary used for
	// signature verification. Empty defaults to "rauc" on $PATH.
	RAUCBinary string
}

// Verify performs cryptographic signature verification of the bundle
// against the installer's keyring. It:
//  1. sanity-checks the bundle exists and is not absurdly small;
//  2. shells out to `rauc verify --keyring=<keyring> <bundle>` which
//     cross-checks the detached CMS/PKCS#7 signature against the
//     trusted X.509 certs in the keyring.
//
// A successful rauc-verify exit implies the bundle was produced by a
// holder of a key whose cert chains to the keyring's trust roots.
func (r *RAUCExtractor) Verify(bundlePath string) error {
	st, err := os.Stat(bundlePath)
	if err != nil {
		return fmt.Errorf("bundle not found: %w", err)
	}
	if st.Size() < 1024*1024 {
		return fmt.Errorf("bundle suspiciously small (%d bytes)", st.Size())
	}

	keyring := r.KeyringPath
	if keyring == "" {
		keyring = DefaultRAUCKeyring
	}
	if _, err := os.Stat(keyring); err != nil {
		return fmt.Errorf("rauc keyring not found at %s: %w", keyring, err)
	}

	rbin := r.RAUCBinary
	if rbin == "" {
		rbin = "rauc"
	}

	if r.Log != nil {
		r.Log("rauc verify %s (keyring=%s, size=%d bytes)", bundlePath, keyring, st.Size())
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
	if err := exe(rbin, "verify", "--keyring="+keyring, bundlePath); err != nil {
		return fmt.Errorf("rauc signature verification failed: %w", err)
	}
	if r.Log != nil {
		r.Log("rauc signature verified for %s", bundlePath)
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
