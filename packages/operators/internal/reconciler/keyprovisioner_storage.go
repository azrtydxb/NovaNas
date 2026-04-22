package reconciler

import (
	"context"
	"fmt"
)

// StorageKeyProvisionerFuncs is a function-pointer adapter that binds
// the operators module's VolumeKeyProvisioner contract to a caller-supplied
// implementation (typically wired at main.go to a *crypto.VolumeKeyManager
// from the storage module). Using function pointers keeps the operators
// module from importing storage/internal/crypto directly — that package
// is marked "internal" and is only accessible to code inside the storage
// module tree.
//
// Typical wire-up (in packages/operators/main.go):
//
//	p := &reconciler.StorageKeyProvisionerFuncs{
//	    Provision: func(ctx context.Context, id string) ([]byte, uint64, error) {
//	        return vmgr.ProvisionVolume(ctx, id)
//	    },
//	    Destroy: func(_ context.Context, id string) error {
//	        vmgr.Unmount(id)
//	        return nil
//	    },
//	}
//
// A nil Provision causes ProvisionVolume to return an explicit error so
// controllers never silently succeed with a placeholder wrapped blob in
// production.
type StorageKeyProvisionerFuncs struct {
	Provision func(ctx context.Context, volumeID string) ([]byte, uint64, error)
	Destroy   func(ctx context.Context, volumeID string) error
}

// ProvisionVolume delegates to the configured Provision function.
func (p *StorageKeyProvisionerFuncs) ProvisionVolume(ctx context.Context, volumeID string) ([]byte, uint64, error) {
	if p == nil || p.Provision == nil {
		return nil, 0, fmt.Errorf("storage key provisioner: no Provision func wired")
	}
	return p.Provision(ctx, volumeID)
}

// DestroyVolume delegates to the configured Destroy function; a nil
// Destroy is treated as a no-op (best-effort cryptographic erase).
func (p *StorageKeyProvisionerFuncs) DestroyVolume(ctx context.Context, volumeID string) error {
	if p == nil || p.Destroy == nil {
		return nil
	}
	return p.Destroy(ctx, volumeID)
}
