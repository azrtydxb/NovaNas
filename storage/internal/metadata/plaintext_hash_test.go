package metadata

import (
	"bytes"
	"testing"
)

// TestVolumeMeta_PlaintextHashRoundtrip verifies Set/Get and the
// unencrypted-chunk sentinel behaviour required by crypto read path.
func TestVolumeMeta_PlaintextHashRoundtrip(t *testing.T) {
	v := &VolumeMeta{VolumeID: "vol-1"}

	// Before any SetChunkPlaintextHash, lookups return (nil, false).
	if h, ok := v.ChunkPlaintextHash("cid-a"); ok || h != nil {
		t.Fatalf("expected (nil, false) for unrecorded chunk, got (%x, %v)", h, ok)
	}

	// Record a hash.
	hashA := bytes.Repeat([]byte{0xAA}, 32)
	v.SetChunkPlaintextHash("cid-a", hashA)

	got, ok := v.ChunkPlaintextHash("cid-a")
	if !ok {
		t.Fatalf("expected ok=true after Set")
	}
	if !bytes.Equal(got, hashA) {
		t.Fatalf("roundtrip mismatch: got %x want %x", got, hashA)
	}

	// Unrecorded chunk id still reports (nil, false) -> treated as
	// unencrypted by the read path.
	if _, ok := v.ChunkPlaintextHash("cid-b"); ok {
		t.Fatalf("expected ok=false for other chunk id")
	}

	// Nil receiver is safe on read.
	var nilv *VolumeMeta
	if _, ok := nilv.ChunkPlaintextHash("cid-a"); ok {
		t.Fatalf("nil receiver must return ok=false")
	}
}
