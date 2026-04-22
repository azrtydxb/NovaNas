// Package chunk — open-chunk state.
//
// Architecture note (A4-Metadata-As-Chunks, docs/02, docs/14 S11/S13/S14):
//
// NovaNas's "everything is chunks" invariant means the metadata service
// itself must live on chunks. Yet BadgerDB's value-log / WAL performs many
// small sequential appends and cannot wait for a full 4 MiB content-addressed
// chunk to fill before acknowledging fsync. To reconcile these, we extend
// the chunk engine with a second state:
//
//   OpenChunk:   mutable, append-only, UUID-identified, capacity-bounded.
//                Every append is fan-out-replicated to the chunk's replica
//                set (same quorum rules as sealed chunks). Reads of an open
//                chunk go through the owner replica.
//
//   SealedChunk: the classic content-addressed, immutable 4 MiB chunk. After
//                sealing, a ChunkID (SHA-256 of the final contents) is
//                computed and registered; subsequent writes are rejected.
//
// Transitions: Open -> (Append, Append, ..., Full || Timeout) -> Sealed.
//
// An open chunk may be sealed early (short sealed chunks are allowed for
// the WAL path). After sealing, the open-chunk UUID is retired and the
// chunk is addressable only by its content hash.
//
// This file defines the Go-side types and in-memory reference
// implementation. The real I/O path lives in the Rust data-plane
// (storage/dataplane/src/chunk). The gRPC surface is in
// proto/novanas/chunk/v1 (OpenChunk, AppendChunk, SealChunk).
package chunk

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"sync"
	"time"

	"github.com/azrtydxb/novanas/storage/internal/crypto"
)

// DefaultOpenChunkCapacity is the default capacity for an open chunk.
// 64 KiB is small enough to keep WAL latency low while amortising the
// seal-and-content-address cost over many entries. Tunable per pool.
const DefaultOpenChunkCapacity = 64 * 1024

// DefaultOpenChunkTimeout is the default wall-clock deadline after which an
// otherwise-idle open chunk is force-sealed even if not full.
const DefaultOpenChunkTimeout = 5 * time.Second

// OpenChunkID identifies an open (mutable) chunk via a random UUID. This is
// distinct from ChunkID (which is the content-addressed SHA-256 of a sealed
// chunk) because an open chunk's contents change with every append.
type OpenChunkID string

// NewOpenChunkID allocates a fresh random UUID-like identifier for an
// open chunk. Uses 16 random bytes rendered as hex (32 chars); sufficient
// to avoid collisions across a cluster's lifetime.
func NewOpenChunkID() (OpenChunkID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generating open-chunk id: %w", err)
	}
	return OpenChunkID(hex.EncodeToString(b[:])), nil
}

// OpenChunkState is the lifecycle state of an open chunk.
type OpenChunkState int

const (
	// OpenChunkOpen is the initial state: appends are accepted.
	OpenChunkOpen OpenChunkState = iota
	// OpenChunkSealed means the chunk has been sealed; it is now immutable
	// and addressable by its content-hash ChunkID.
	OpenChunkSealed
)

// OpenChunk is a mutable, append-only chunk in flight.
//
// Concurrency: OpenChunk is safe for concurrent Append/Seal callers. In
// the real data-plane the replication fan-out happens before Append
// returns; here we model just the local buffer state.
type OpenChunk struct {
	mu sync.Mutex

	id         OpenChunkID
	poolID     string
	capacity   int
	createdAt  time.Time
	lastAppend time.Time
	buf        []byte
	state      OpenChunkState
	sealedAs   ChunkID // set once state == OpenChunkSealed
}

// NewOpenChunk allocates an open chunk with the given capacity (bytes).
// capacity must be > 0 and <= ChunkSize.
func NewOpenChunk(poolID string, capacity int) (*OpenChunk, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("open-chunk capacity must be > 0, got %d", capacity)
	}
	if capacity > ChunkSize {
		return nil, fmt.Errorf("open-chunk capacity %d exceeds ChunkSize %d", capacity, ChunkSize)
	}
	id, err := NewOpenChunkID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	return &OpenChunk{
		id:         id,
		poolID:     poolID,
		capacity:   capacity,
		createdAt:  now,
		lastAppend: now,
		buf:        make([]byte, 0, capacity),
		state:      OpenChunkOpen,
	}, nil
}

// ID returns the open-chunk UUID.
func (o *OpenChunk) ID() OpenChunkID { return o.id }

// PoolID returns the pool this open chunk belongs to.
func (o *OpenChunk) PoolID() string { return o.poolID }

// Capacity returns the byte capacity of this open chunk.
func (o *OpenChunk) Capacity() int { return o.capacity }

// Len returns the current byte offset (how much has been appended).
func (o *OpenChunk) Len() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.buf)
}

// State returns the current lifecycle state.
func (o *OpenChunk) State() OpenChunkState {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.state
}

// SealedID returns the content-addressed ChunkID this open chunk was
// sealed as, or the empty string if it is still open.
func (o *OpenChunk) SealedID() ChunkID {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.sealedAs
}

// Errors.
var (
	ErrOpenChunkSealed       = errors.New("open chunk is already sealed")
	ErrOpenChunkFull         = errors.New("open chunk is full")
	ErrOpenChunkOffsetMismatch = errors.New("append offset does not match current length")
	ErrOpenChunkNotFound     = errors.New("open chunk not found")
)

// Append writes data at the given byte offset. The offset MUST equal the
// current length (appends are strictly append-only — no holes, no
// overwrites). This constraint lets replicas detect lost writes without
// a per-byte log.
//
// Returns ErrOpenChunkSealed if the chunk has been sealed,
// ErrOpenChunkFull if the append would exceed capacity,
// ErrOpenChunkOffsetMismatch if offset != current length.
func (o *OpenChunk) Append(offset int, data []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.state == OpenChunkSealed {
		return ErrOpenChunkSealed
	}
	if offset != len(o.buf) {
		return fmt.Errorf("%w: got offset=%d, current length=%d",
			ErrOpenChunkOffsetMismatch, offset, len(o.buf))
	}
	if len(o.buf)+len(data) > o.capacity {
		return ErrOpenChunkFull
	}
	o.buf = append(o.buf, data...)
	o.lastAppend = time.Now()
	return nil
}

// Snapshot returns a copy of the current buffer contents. Intended for
// reads while the chunk is still open (e.g. crash recovery).
func (o *OpenChunk) Snapshot() []byte {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]byte, len(o.buf))
	copy(out, o.buf)
	return out
}

// Seal transitions Open -> Sealed. Computes the content hash over the
// currently-appended bytes, records it, and returns the resulting
// sealed Chunk. After Seal, further Append calls return
// ErrOpenChunkSealed.
//
// Sealing a zero-length open chunk is allowed (produces the hash of an
// empty byte slice); callers that want to reject empty seals should
// check Len() first.
func (o *OpenChunk) Seal() (*Chunk, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.state == OpenChunkSealed {
		return nil, ErrOpenChunkSealed
	}
	sealedData := make([]byte, len(o.buf))
	copy(sealedData, o.buf)

	id := NewChunkID(sealedData)
	table := crc32.MakeTable(crc32.Castagnoli)
	c := &Chunk{
		ID:       id,
		Data:     sealedData,
		Checksum: crc32.Checksum(sealedData, table),
	}
	o.state = OpenChunkSealed
	o.sealedAs = id
	return c, nil
}

// SealEncrypted transitions Open -> Sealed with convergent encryption.
// The accumulated plaintext is encrypted under dk (32 bytes) before
// the content hash is computed, so the resulting ChunkID is
// SHA-256(ciphertext||auth_tag). This preserves dedup within the DK
// scope while guaranteeing data-at-rest encryption from the moment
// the chunk is sealed.
//
// For the open-chunk path we encrypt at seal time (rather than
// per-append) for simplicity: the buffer is small (64 KiB default)
// and seal-time encryption avoids the complexity of a streaming AEAD
// across many small appends.
func (o *OpenChunk) SealEncrypted(dk []byte) (*Chunk, error) {
	if len(dk) != crypto.ChunkKeySize {
		return nil, fmt.Errorf("open-chunk: dataset key must be %d bytes, got %d", crypto.ChunkKeySize, len(dk))
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.state == OpenChunkSealed {
		return nil, ErrOpenChunkSealed
	}
	sealedData := make([]byte, len(o.buf))
	copy(sealedData, o.buf)

	enc, err := crypto.EncryptChunk(dk, sealedData)
	if err != nil {
		return nil, fmt.Errorf("open-chunk: encrypt: %w", err)
	}
	id := ChunkID(hex.EncodeToString(enc.ChunkID[:]))
	table := crc32.MakeTable(crc32.Castagnoli)
	c := &Chunk{
		ID:            id,
		Data:          enc.Ciphertext,
		Checksum:      crc32.Checksum(enc.Ciphertext, table),
		Encrypted:     true,
		AuthTag:       enc.AuthTag,
		PlaintextHash: enc.PlaintextHash,
	}
	o.state = OpenChunkSealed
	o.sealedAs = id
	return c, nil
}

// ShouldSeal reports whether the open chunk should be sealed given its
// current fill level and an optional idle timeout. Sealing happens when
// the chunk is full (len == capacity) OR it has been idle longer than
// timeout (timeout == 0 disables the timeout check).
func (o *OpenChunk) ShouldSeal(timeout time.Duration) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.state == OpenChunkSealed {
		return false
	}
	if len(o.buf) >= o.capacity {
		return true
	}
	if timeout > 0 && time.Since(o.lastAppend) >= timeout {
		return true
	}
	return false
}

// OpenChunkRegistry is an in-memory tracker for open chunks. The real
// data-plane keeps this in the Rust chunk engine; this Go-side registry
// is used by management-plane components and tests.
type OpenChunkRegistry struct {
	mu     sync.Mutex
	chunks map[OpenChunkID]*OpenChunk
}

// NewOpenChunkRegistry constructs an empty registry.
func NewOpenChunkRegistry() *OpenChunkRegistry {
	return &OpenChunkRegistry{chunks: make(map[OpenChunkID]*OpenChunk)}
}

// Open allocates a new open chunk and registers it.
func (r *OpenChunkRegistry) Open(poolID string, capacity int) (*OpenChunk, error) {
	c, err := NewOpenChunk(poolID, capacity)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.chunks[c.id] = c
	r.mu.Unlock()
	return c, nil
}

// Get returns the open chunk with the given id, or ErrOpenChunkNotFound.
func (r *OpenChunkRegistry) Get(id OpenChunkID) (*OpenChunk, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.chunks[id]
	if !ok {
		return nil, ErrOpenChunkNotFound
	}
	return c, nil
}

// Append is a convenience that looks up the open chunk and appends.
func (r *OpenChunkRegistry) Append(id OpenChunkID, offset int, data []byte) error {
	c, err := r.Get(id)
	if err != nil {
		return err
	}
	return c.Append(offset, data)
}

// Seal seals the open chunk, removes it from the open registry, and
// returns the resulting sealed Chunk.
func (r *OpenChunkRegistry) Seal(id OpenChunkID) (*Chunk, error) {
	c, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	sealed, err := c.Seal()
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	delete(r.chunks, id)
	r.mu.Unlock()
	return sealed, nil
}

// SealEncrypted seals the open chunk with convergent encryption under
// the supplied dataset key, removes it from the registry, and returns
// the resulting encrypted sealed Chunk.
func (r *OpenChunkRegistry) SealEncrypted(id OpenChunkID, dk []byte) (*Chunk, error) {
	c, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	sealed, err := c.SealEncrypted(dk)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	delete(r.chunks, id)
	r.mu.Unlock()
	return sealed, nil
}

// List returns the ids of all currently-open chunks.
func (r *OpenChunkRegistry) List() []OpenChunkID {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]OpenChunkID, 0, len(r.chunks))
	for id := range r.chunks {
		out = append(out, id)
	}
	return out
}
