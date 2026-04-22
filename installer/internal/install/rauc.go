// Package install runs the post-selection install pipeline.
package install

import (
	"encoding/binary"
	"fmt"
	"io"
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
// Verify now performs an in-process CMS/PKCS#7 verification of classic
// RAUC bundles (squashfs + appended detached signature) using
// crypto/x509 + go.mozilla.org/pkcs7. If the in-process path fails (e.g.
// verity-format bundle, or unsupported signature layout) it falls back
// to shelling out to `rauc verify`.
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
	// DisableInProcessVerify forces the shellout path. Mostly useful
	// for tests that want to exercise the fallback branch.
	DisableInProcessVerify bool
}

// raucClassicSignatureLayout describes the trailer of a classic RAUC
// bundle: the final 4 bytes are a big-endian uint32 giving the size of
// the immediately preceding detached CMS signature.
//
// Layout:
//
//	[ squashfs content ... ][ CMS SignedData (DER) ][ uint32 sig_size (BE) ]
//
// Note: upstream RAUC uses BIG-endian for the trailing size (see
// librauc/src/bundle.c). The covered content is everything from the
// start of the file up to (but not including) the signature blob.
func readRAUCClassicSignature(path string) (content []byte, sig []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	total := st.Size()
	if total < 4 {
		return nil, nil, fmt.Errorf("bundle too small to carry signature trailer")
	}
	// Read trailer uint32 (big-endian).
	var sizeBuf [4]byte
	if _, err := f.ReadAt(sizeBuf[:], total-4); err != nil {
		return nil, nil, fmt.Errorf("read trailer: %w", err)
	}
	sigSize := int64(binary.BigEndian.Uint32(sizeBuf[:]))
	if sigSize <= 0 || sigSize > total-4 {
		return nil, nil, fmt.Errorf("implausible signature size %d", sigSize)
	}
	contentEnd := total - 4 - sigSize
	// Read signature.
	sig = make([]byte, sigSize)
	if _, err := f.ReadAt(sig, contentEnd); err != nil {
		return nil, nil, fmt.Errorf("read signature: %w", err)
	}
	// Read content (for reasonably-sized bundles this fits in memory;
	// for very large bundles callers should hash the content in a stream
	// instead — a future optimisation).
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, nil, err
	}
	content = make([]byte, contentEnd)
	if _, err := io.ReadFull(f, content); err != nil {
		return nil, nil, fmt.Errorf("read content: %w", err)
	}
	return content, sig, nil
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

	// 1) In-process verification via crypto/x509 + PKCS#7. Works for the
	//    classic RAUC bundle layout (squashfs + appended detached CMS
	//    signature, with a big-endian uint32 trailer giving the sig size).
	//    Falls through to shellout on any failure; the shellout handles
	//    verity-format bundles and any signature layouts we don't yet
	//    parse in-process.
	if !r.DisableInProcessVerify {
		content, sig, err := readRAUCClassicSignature(bundlePath)
		if err == nil {
			if verr := verifyPKCS7Detached(keyring, sig, content); verr == nil {
				if r.Log != nil {
					r.Log("rauc signature verified in-process (pkcs7) for %s", bundlePath)
				}
				return nil
			} else if r.Log != nil {
				r.Log("in-process pkcs7 verify failed, falling back to rauc binary: %v", verr)
			}
		} else if r.Log != nil {
			r.Log("rauc classic signature trailer not readable (%v); using rauc binary", err)
		}
	}

	// 2) Shellout fallback.
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
		r.Log("rauc signature verified (shellout) for %s", bundlePath)
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
