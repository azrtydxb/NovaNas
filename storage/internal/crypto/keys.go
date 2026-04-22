package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"sync"
)

// DataKey wraps a 32-byte AES-256 dataset key plus identity metadata.
// DataKey is intentionally non-comparable (contains a mutex) and its
// raw bytes are accessed only via Bytes(). Close() zeroises the key
// material; after Close the DataKey is unusable.
type DataKey struct {
	mu      sync.Mutex
	id      string // stable identifier (volume UUID or equivalent)
	version uint64 // master-key version at wrap time
	raw     [ChunkKeySize]byte
	closed  bool
}

// NewDataKey constructs a DataKey from raw 32 bytes. The caller's copy
// is NOT zeroised — callers holding the raw bytes should zero their
// own copy once the DataKey is constructed.
func NewDataKey(id string, version uint64, raw []byte) (*DataKey, error) {
	if len(raw) != ChunkKeySize {
		return nil, fmt.Errorf("crypto: dataset key must be %d bytes, got %d", ChunkKeySize, len(raw))
	}
	dk := &DataKey{id: id, version: version}
	copy(dk.raw[:], raw)
	return dk, nil
}

// GenerateDataKey creates a fresh random DataKey. version is typically
// set by the caller to the master-key version returned by OpenBao.
func GenerateDataKey(id string, version uint64) (*DataKey, error) {
	var raw [ChunkKeySize]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, fmt.Errorf("crypto: generate dk: %w", err)
	}
	return NewDataKey(id, version, raw[:])
}

// ID returns the stable identifier.
func (d *DataKey) ID() string { return d.id }

// Version returns the master-key version at the time of wrap.
func (d *DataKey) Version() uint64 { return d.version }

// Bytes returns a defensive copy of the raw key. Callers must zeroise
// the returned slice when done (use ZeroBytes).
func (d *DataKey) Bytes() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, errors.New("crypto: data key is closed")
	}
	out := make([]byte, ChunkKeySize)
	copy(out, d.raw[:])
	return out, nil
}

// Equal reports whether two DataKeys carry the same key material. Uses
// constant-time comparison.
func (d *DataKey) Equal(other *DataKey) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if other == nil {
		return false
	}
	other.mu.Lock()
	defer other.mu.Unlock()
	if d.closed || other.closed {
		return false
	}
	return subtle.ConstantTimeCompare(d.raw[:], other.raw[:]) == 1
}

// Close zeroises the key material. Subsequent Bytes() calls fail.
func (d *DataKey) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	for i := range d.raw {
		d.raw[i] = 0
	}
	d.closed = true
}

// ZeroBytes zeroises a byte slice in place. Call on defensive copies
// returned by DataKey.Bytes().
func ZeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// KeyCache holds unwrapped DataKeys in memory for the lifetime of a
// mount. Keyed by volume id. Eviction zeroises the key material.
//
// Concurrency: internally uses a sync.RWMutex; Get / Put / Evict are
// all safe to call from multiple goroutines.
type KeyCache struct {
	mu   sync.RWMutex
	keys map[string]*DataKey
}

// NewKeyCache constructs an empty cache.
func NewKeyCache() *KeyCache {
	return &KeyCache{keys: make(map[string]*DataKey)}
}

// Put inserts or replaces the DataKey for volumeID. If an existing
// entry is present it is Close()d first so its bytes are zeroised.
func (c *KeyCache) Put(volumeID string, dk *DataKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.keys[volumeID]; ok && old != dk {
		old.Close()
	}
	c.keys[volumeID] = dk
}

// Get returns the cached DataKey for volumeID, or (nil, false).
func (c *KeyCache) Get(volumeID string) (*DataKey, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	dk, ok := c.keys[volumeID]
	if !ok {
		return nil, false
	}
	return dk, true
}

// Evict removes and Close()s the DataKey for volumeID (zeroising the
// key material). Returns true if an entry was removed.
func (c *KeyCache) Evict(volumeID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	dk, ok := c.keys[volumeID]
	if !ok {
		return false
	}
	dk.Close()
	delete(c.keys, volumeID)
	return true
}

// Len reports the number of cached entries.
func (c *KeyCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.keys)
}

// Close evicts and zeroises every entry.
func (c *KeyCache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, dk := range c.keys {
		dk.Close()
		delete(c.keys, id)
	}
}
