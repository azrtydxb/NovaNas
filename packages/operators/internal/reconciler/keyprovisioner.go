package reconciler

import "context"

// VolumeKeyProvisioner is the minimal contract NovaNas controllers use to
// obtain an encrypted volume's wrapped Dataset Key at provision time.
//
// It is intentionally defined in the operators module (not imported from
// storage/internal/crypto) so this module does not pick up a cross-module
// Go dependency on the storage engine. The storage engine exposes a
// *crypto.VolumeKeyManager whose Provision method can be adapted to this
// interface via a tiny shim at wire-up time.
//
// Behaviour:
//   - ProvisionVolume generates a fresh 32-byte Dataset Key, wraps it via
//     OpenBao Transit, caches the raw key in the engine's KeyCache, and
//     returns (wrappedBlob, masterKeyVersion) which the caller MUST persist
//     in the CR's status. The wrapped blob is the only artefact that
//     survives a controller restart -- the raw key is re-derived on
//     volume Mount.
//   - DestroyVolume is optional "cryptographic erase": if supported by
//     the backend it deletes the wrapped DK, after which the volume's
//     chunks become un-decryptable. Implementations that do not support
//     destroy should return (nil) and log an explicit warning.
type VolumeKeyProvisioner interface {
	// ProvisionVolume creates + wraps a new DK for volumeID.
	ProvisionVolume(ctx context.Context, volumeID string) (wrapped []byte, keyVersion uint64, err error)
	// DestroyVolume optionally evicts the wrapped DK.
	DestroyVolume(ctx context.Context, volumeID string) error
}

// NoopKeyProvisioner is a stand-in used when the manager is started
// without encryption wiring (e.g. integration tests). Its
// ProvisionVolume returns a deterministic placeholder wrapped blob so
// controllers can still update status; DestroyVolume is a no-op.
//
// Production builds must inject a real provisioner backed by
// crypto.VolumeKeyManager.
type NoopKeyProvisioner struct{}

// ProvisionVolume returns a deterministic placeholder wrapped blob.
func (NoopKeyProvisioner) ProvisionVolume(_ context.Context, volumeID string) ([]byte, uint64, error) {
	return []byte("noop-wrapped-dk:" + volumeID), 1, nil
}

// DestroyVolume is a no-op in the Noop implementation.
func (NoopKeyProvisioner) DestroyVolume(_ context.Context, _ string) error { return nil }
