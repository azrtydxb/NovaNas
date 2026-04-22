package crypto

import (
	"context"
	"fmt"

	"github.com/azrtydxb/novanas/storage/internal/openbao"
)

// VolumeKeyManager couples an OpenBao Transit client with a local
// KeyCache. It is the single choke point through which the chunk
// engine fetches per-volume Dataset Keys.
//
// Lifetime model:
//   - ProvisionVolume: called at volume-create time. Generates a fresh
//     DK, wraps it with Transit, returns the wrapped blob + version to
//     the caller to persist in volume metadata. Also inserts the raw
//     DK into the cache so the create path can immediately write
//     chunks.
//   - Mount: called at mount time. Unwraps the supplied blob via
//     Transit and caches the raw DK.
//   - Unmount: evicts the cached DK (zeroising the key material).
type VolumeKeyManager struct {
	transit       openbao.TransitClient
	masterKeyName string
	cache         *KeyCache
}

// NewVolumeKeyManager constructs a manager. masterKeyName is the
// OpenBao Transit key identifier (e.g. "novanas/chunk-master"). An
// empty cache is allocated.
func NewVolumeKeyManager(transit openbao.TransitClient, masterKeyName string) *VolumeKeyManager {
	return &VolumeKeyManager{
		transit:       transit,
		masterKeyName: masterKeyName,
		cache:         NewKeyCache(),
	}
}

// MasterKeyName returns the configured Transit master-key name.
func (m *VolumeKeyManager) MasterKeyName() string { return m.masterKeyName }

// Cache exposes the underlying KeyCache, mainly for tests / metrics.
func (m *VolumeKeyManager) Cache() *KeyCache { return m.cache }

// ProvisionVolume generates a fresh 32-byte Dataset Key for the given
// volume, wraps it via OpenBao Transit, caches the raw key, and
// returns (wrappedBlob, masterKeyVersion).
func (m *VolumeKeyManager) ProvisionVolume(ctx context.Context, volumeID string) ([]byte, uint64, error) {
	dk, err := GenerateDataKey(volumeID, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("crypto: generate dk: %w", err)
	}
	raw, err := dk.Bytes()
	if err != nil {
		return nil, 0, err
	}
	defer ZeroBytes(raw)

	wrapped, version, err := m.transit.WrapDK(ctx, m.masterKeyName, raw)
	if err != nil {
		dk.Close()
		return nil, 0, fmt.Errorf("crypto: wrap dk: %w", err)
	}
	// Rewrap the DataKey with the correct version metadata.
	withVersion, err := NewDataKey(volumeID, version, raw)
	if err != nil {
		dk.Close()
		return nil, 0, err
	}
	dk.Close()
	m.cache.Put(volumeID, withVersion)
	return wrapped, version, nil
}

// Mount unwraps the supplied wrapped blob and caches the DK.
func (m *VolumeKeyManager) Mount(ctx context.Context, volumeID string, wrapped []byte, recordedVersion uint64) error {
	if dk, ok := m.cache.Get(volumeID); ok && dk != nil {
		return nil // already mounted
	}
	raw, err := m.transit.UnwrapDK(ctx, m.masterKeyName, wrapped)
	if err != nil {
		return fmt.Errorf("crypto: unwrap dk: %w", err)
	}
	defer ZeroBytes(raw)
	dk, err := NewDataKey(volumeID, recordedVersion, raw)
	if err != nil {
		return err
	}
	m.cache.Put(volumeID, dk)
	return nil
}

// Unmount evicts the cached DK (zeroising key material).
func (m *VolumeKeyManager) Unmount(volumeID string) { m.cache.Evict(volumeID) }

// Get returns the cached DataKey for a mounted volume. Returns
// (nil, false) if the volume is not mounted. Callers should treat
// the returned DataKey as borrowed — do not Close() it.
func (m *VolumeKeyManager) Get(volumeID string) (*DataKey, bool) {
	return m.cache.Get(volumeID)
}

// Close evicts every cached DK.
func (m *VolumeKeyManager) Close() { m.cache.Close() }
