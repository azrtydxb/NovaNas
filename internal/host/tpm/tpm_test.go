package tpm

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/go-tpm/tpm2"
	"github.com/google/go-tpm/tpm2/transport"
	"github.com/google/go-tpm/tpm2/transport/simulator"
)

// newSimSealer returns a Sealer wired to a fresh in-process TPM
// simulator. The simulator is closed at the end of the test. If the
// simulator cannot be opened (e.g. CGO disabled at build time), the
// test is skipped — wire-format coverage in wire_test.go is enough.
func newSimSealer(t *testing.T, pcrs []int) *Sealer {
	t.Helper()
	sim, err := simulator.OpenSimulator()
	if err != nil {
		t.Skipf("TPM simulator unavailable: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })

	s := &Sealer{
		PCRs: pcrs,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		// Force the transient-SRK path; the simulator has no
		// pre-provisioned persistent SRK.
		OwnerHandle: 0,
		open: func() (transport.TPMCloser, error) {
			// Wrap the long-lived simulator in a no-op closer so
			// each Sealer call doesn't actually shut it down — we
			// reset PCRs / state via the simulator handle directly.
			return noopCloser{sim}, nil
		},
	}
	return s
}

type noopCloser struct {
	transport.TPMCloser
}

func (n noopCloser) Close() error { return nil }

func TestSealUnsealRoundTrip(t *testing.T) {
	s := newSimSealer(t, []int{7})

	plaintext := []byte("super-secret-dek-32-bytes-aaaaa!")
	blob, err := s.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if len(blob) < 16 {
		t.Fatalf("blob suspiciously short: %d bytes", len(blob))
	}
	if string(blob[:4]) != wireMagic {
		t.Errorf("blob does not start with magic")
	}

	got, err := s.Unseal(blob)
	if err != nil {
		t.Fatalf("Unseal: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("got %q want %q", got, plaintext)
	}
}

func TestSealUnsealMultiplePCRs(t *testing.T) {
	s := newSimSealer(t, []int{0, 2, 7})

	plaintext := []byte("32-byte-dek-with-three-pcrs-bind")
	blob, err := s.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	got, err := s.Unseal(blob)
	if err != nil {
		t.Fatalf("Unseal: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("got %q want %q", got, plaintext)
	}
}

func TestUnsealFailsAfterPCRChange(t *testing.T) {
	s := newSimSealer(t, []int{16}) // PCR 16 is debug, freely extendable

	plaintext := []byte("dek-bound-to-debug-pcr-16-12345!")
	blob, err := s.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	// Unseal once to confirm the baseline works.
	if _, err := s.Unseal(blob); err != nil {
		t.Fatalf("baseline Unseal: %v", err)
	}

	// Now extend PCR 16 to change its value, then expect Unseal to
	// fail with ErrPCRMismatch.
	tpmConn, err := s.open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer tpmConn.Close()
	if _, err := (&tpm2.PCRExtend{
		PCRHandle: tpm2.AuthHandle{
			Handle: tpm2.TPMHandle(16),
			Auth:   tpm2.PasswordAuth(nil),
		},
		Digests: tpm2.TPMLDigestValues{
			Digests: []tpm2.TPMTHA{{
				HashAlg: tpm2.TPMAlgSHA256,
				Digest:  bytes.Repeat([]byte{0xaa}, 32),
			}},
		},
	}).Execute(tpmConn); err != nil {
		t.Fatalf("PCRExtend: %v", err)
	}

	_, err = s.Unseal(blob)
	if err == nil {
		t.Fatal("expected Unseal to fail after PCR change")
	}
	if !errors.Is(err, ErrPCRMismatch) {
		t.Errorf("want ErrPCRMismatch, got %v", err)
	}
}

func TestSealRejectsEmpty(t *testing.T) {
	s := newSimSealer(t, []int{7})
	if _, err := s.Seal(nil); err == nil {
		t.Error("expected error for empty plaintext")
	}
}

func TestSealRejectsOversize(t *testing.T) {
	s := newSimSealer(t, []int{7})
	if _, err := s.Seal(bytes.Repeat([]byte{0x42}, 200)); err == nil {
		t.Error("expected error for oversize plaintext")
	}
}

func TestUnsealRejectsGarbage(t *testing.T) {
	s := newSimSealer(t, []int{7})
	if _, err := s.Unseal([]byte("not a real blob")); err == nil {
		t.Error("expected error for garbage blob")
	}
}
