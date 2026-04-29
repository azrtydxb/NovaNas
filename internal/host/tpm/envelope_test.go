package tpm

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"
)

// fakeSealer simulates TPM seal/unseal without hardware. Each call to
// Seal returns the input prefixed with a 4-byte magic so Unseal can
// reject blobs that came from another sealer instance.
type fakeSealer struct {
	id        byte
	failSeal  error
	failOpen  error
	pcrChange bool
}

var fakeMagic = []byte{0xFA, 0xCE, 0xCA, 0xFE}

func (f *fakeSealer) Seal(p []byte) ([]byte, error) {
	if f.failSeal != nil {
		return nil, f.failSeal
	}
	out := make([]byte, 0, 4+1+len(p))
	out = append(out, fakeMagic...)
	out = append(out, f.id)
	out = append(out, p...)
	return out, nil
}

func (f *fakeSealer) Unseal(blob []byte) ([]byte, error) {
	if f.failOpen != nil {
		return nil, f.failOpen
	}
	if f.pcrChange {
		return nil, ErrPCRMismatch
	}
	if len(blob) < 5 || !bytes.Equal(blob[:4], fakeMagic) {
		return nil, errors.New("fake: bad magic")
	}
	if blob[4] != f.id {
		return nil, errors.New("fake: wrong sealer id")
	}
	return blob[5:], nil
}

func TestWrapUnwrap_RoundTrip(t *testing.T) {
	s := &fakeSealer{id: 1}
	for _, size := range []int{0, 1, 32, 1023, 65536} {
		pt := make([]byte, size)
		_, _ = rand.Read(pt)
		blob, err := WrapAEAD(pt, s)
		if err != nil {
			t.Fatalf("wrap %d: %v", size, err)
		}
		got, err := UnwrapAEAD(blob, s)
		if err != nil {
			t.Fatalf("unwrap %d: %v", size, err)
		}
		if !bytes.Equal(pt, got) {
			t.Fatalf("size=%d: roundtrip mismatch", size)
		}
	}
}

func TestUnwrap_RejectsTruncatedBlob(t *testing.T) {
	s := &fakeSealer{id: 1}
	blob, err := WrapAEAD([]byte("hello"), s)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		b    []byte
	}{
		{"empty", []byte{}},
		{"one byte", []byte{0x00}},
		{"truncated DEK", blob[:len(blob)-len(blob)/2]},
		{"chopped tag", blob[:len(blob)-1]},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := UnwrapAEAD(c.b, s); err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
		})
	}
}

func TestUnwrap_RejectsTamperedCiphertext(t *testing.T) {
	s := &fakeSealer{id: 1}
	blob, err := WrapAEAD([]byte("hello world"), s)
	if err != nil {
		t.Fatal(err)
	}
	// Flip a bit in the last byte (inside the GCM tag) — must fail.
	tampered := append([]byte(nil), blob...)
	tampered[len(tampered)-1] ^= 0x01
	if _, err := UnwrapAEAD(tampered, s); err == nil {
		t.Fatal("expected GCM auth failure on tampered tag")
	}
}

func TestUnwrap_PCRMismatchPropagates(t *testing.T) {
	wrap := &fakeSealer{id: 1}
	blob, err := WrapAEAD([]byte("data"), wrap)
	if err != nil {
		t.Fatal(err)
	}
	unseal := &fakeSealer{id: 1, pcrChange: true}
	_, err = UnwrapAEAD(blob, unseal)
	if !errors.Is(err, ErrPCRMismatch) {
		t.Fatalf("expected ErrPCRMismatch, got %v", err)
	}
}

func TestWrap_PropagatesSealError(t *testing.T) {
	s := &fakeSealer{failSeal: errors.New("tpm offline")}
	if _, err := WrapAEAD([]byte("x"), s); err == nil {
		t.Fatal("expected wrap to fail when sealer.Seal fails")
	}
}
