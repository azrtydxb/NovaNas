package chunk

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

func TestNewOpenChunk_InvalidCapacity(t *testing.T) {
	if _, err := NewOpenChunk("p1", 0); err == nil {
		t.Error("capacity 0 should error")
	}
	if _, err := NewOpenChunk("p1", ChunkSize+1); err == nil {
		t.Error("capacity > ChunkSize should error")
	}
}

func TestOpenChunk_AppendAndSeal(t *testing.T) {
	o, err := NewOpenChunk("pool-meta", 32)
	if err != nil {
		t.Fatalf("NewOpenChunk: %v", err)
	}
	if o.State() != OpenChunkOpen {
		t.Fatalf("state = %v, want Open", o.State())
	}
	if o.Len() != 0 {
		t.Fatalf("initial len = %d, want 0", o.Len())
	}

	if err := o.Append(0, []byte("hello ")); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := o.Append(6, []byte("world")); err != nil {
		t.Fatalf("second append: %v", err)
	}
	if o.Len() != 11 {
		t.Fatalf("len after appends = %d, want 11", o.Len())
	}
	if got := o.Snapshot(); !bytes.Equal(got, []byte("hello world")) {
		t.Fatalf("snapshot = %q, want 'hello world'", got)
	}

	sealed, err := o.Seal()
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if o.State() != OpenChunkSealed {
		t.Fatalf("state after seal = %v, want Sealed", o.State())
	}
	want := sha256.Sum256([]byte("hello world"))
	if sealed.ID != ChunkID(hex.EncodeToString(want[:])) {
		t.Fatalf("sealed ID = %s, want %s", sealed.ID, hex.EncodeToString(want[:]))
	}
	if o.SealedID() != sealed.ID {
		t.Fatalf("SealedID() = %s, want %s", o.SealedID(), sealed.ID)
	}
	if err := sealed.VerifyChecksum(); err != nil {
		t.Fatalf("sealed chunk checksum: %v", err)
	}
}

func TestOpenChunk_OffsetMismatch(t *testing.T) {
	o, _ := NewOpenChunk("p", 32)
	_ = o.Append(0, []byte("abc"))
	if err := o.Append(0, []byte("x")); !errors.Is(err, ErrOpenChunkOffsetMismatch) {
		t.Fatalf("want ErrOpenChunkOffsetMismatch, got %v", err)
	}
	if err := o.Append(10, []byte("x")); !errors.Is(err, ErrOpenChunkOffsetMismatch) {
		t.Fatalf("want ErrOpenChunkOffsetMismatch for beyond-end, got %v", err)
	}
}

func TestOpenChunk_Full(t *testing.T) {
	o, _ := NewOpenChunk("p", 4)
	if err := o.Append(0, []byte("abcd")); err != nil {
		t.Fatalf("fill append: %v", err)
	}
	if err := o.Append(4, []byte("e")); !errors.Is(err, ErrOpenChunkFull) {
		t.Fatalf("want ErrOpenChunkFull, got %v", err)
	}
}

func TestOpenChunk_SealedImmutable(t *testing.T) {
	o, _ := NewOpenChunk("p", 32)
	_ = o.Append(0, []byte("data"))
	_, _ = o.Seal()
	if err := o.Append(4, []byte("x")); !errors.Is(err, ErrOpenChunkSealed) {
		t.Fatalf("want ErrOpenChunkSealed after seal, got %v", err)
	}
	if _, err := o.Seal(); !errors.Is(err, ErrOpenChunkSealed) {
		t.Fatalf("double-seal should error, got %v", err)
	}
}

func TestOpenChunk_ShouldSeal(t *testing.T) {
	o, _ := NewOpenChunk("p", 4)
	if o.ShouldSeal(time.Hour) {
		t.Error("empty chunk should not need sealing")
	}
	_ = o.Append(0, []byte("abcd"))
	if !o.ShouldSeal(0) {
		t.Error("full chunk should need sealing")
	}

	o2, _ := NewOpenChunk("p", 16)
	_ = o2.Append(0, []byte("x"))
	time.Sleep(5 * time.Millisecond)
	if !o2.ShouldSeal(1 * time.Millisecond) {
		t.Error("idle chunk past timeout should need sealing")
	}
}

func TestOpenChunkRegistry_Lifecycle(t *testing.T) {
	reg := NewOpenChunkRegistry()
	c, err := reg.Open("pool-meta", 64)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	id := c.ID()

	if err := reg.Append(id, 0, []byte("abc")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := reg.Append(id, 3, []byte("def")); err != nil {
		t.Fatalf("Append 2: %v", err)
	}

	got, err := reg.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got.Snapshot(), []byte("abcdef")) {
		t.Fatalf("snapshot = %q", got.Snapshot())
	}

	sealed, err := reg.Seal(id)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	want := sha256.Sum256([]byte("abcdef"))
	if sealed.ID != ChunkID(hex.EncodeToString(want[:])) {
		t.Fatalf("sealed id mismatch")
	}

	if _, err := reg.Get(id); !errors.Is(err, ErrOpenChunkNotFound) {
		t.Fatalf("get after seal should be not-found, got %v", err)
	}
}

func TestNewOpenChunkID_Unique(t *testing.T) {
	seen := make(map[OpenChunkID]struct{})
	for i := 0; i < 100; i++ {
		id, err := NewOpenChunkID()
		if err != nil {
			t.Fatalf("NewOpenChunkID: %v", err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate open-chunk id %s", id)
		}
		seen[id] = struct{}{}
	}
}
