package reconciler

import (
	"context"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CryptoFinalizerName is the finalizer attached to encrypted volume CRs
// (BlockVolume / Dataset / Bucket) so that on delete the controller can
// call VolumeKeyProvisioner.DestroyVolume before Kubernetes removes the
// object. This provides cryptographic erase on delete: once the wrapped
// DK is destroyed in OpenBao / evicted from the key cache, the volume's
// chunks (still on disk until GC) become un-decryptable.
const CryptoFinalizerName = "novanas.io/crypto-finalizer"

// EnsureCryptoFinalizer adds the NovaNas crypto finalizer to obj if it
// is not already present, and persists the update via c.Update. Safe to
// call on every reconcile -- it short-circuits when the finalizer is
// already in place. Objects that are already being deleted are left
// alone (removal is handled by HandleCryptoFinalizerOnDelete).
func EnsureCryptoFinalizer(ctx context.Context, c client.Client, obj client.Object) (bool, error) {
	if !obj.GetDeletionTimestamp().IsZero() {
		return false, nil
	}
	fin := obj.GetFinalizers()
	if slices.Contains(fin, CryptoFinalizerName) {
		return false, nil
	}
	obj.SetFinalizers(append(fin, CryptoFinalizerName))
	if err := c.Update(ctx, obj); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveCryptoFinalizer removes the NovaNas crypto finalizer from obj
// and persists the update. Returns whether it was actually removed.
func RemoveCryptoFinalizer(ctx context.Context, c client.Client, obj client.Object) (bool, error) {
	fin := obj.GetFinalizers()
	if !slices.Contains(fin, CryptoFinalizerName) {
		return false, nil
	}
	out := make([]string, 0, len(fin))
	for _, f := range fin {
		if f == CryptoFinalizerName {
			continue
		}
		out = append(out, f)
	}
	obj.SetFinalizers(out)
	if err := c.Update(ctx, obj); err != nil {
		return false, err
	}
	return true, nil
}

// HandleCryptoFinalizerOnDelete implements the delete-side of the
// crypto finalizer. When obj has a non-zero deletion timestamp and
// carries the crypto finalizer, it calls kp.DestroyVolume(volumeID) and
// then removes the finalizer so Kubernetes can GC the object.
//
// Returns (handled, err). When handled is true the caller should return
// from its reconcile immediately. When handled is false no delete
// action was required and the normal reconcile flow should continue.
//
// kp may be nil: in that case the finalizer is removed without any
// cryptographic erase. This path is only used when encryption was never
// wired; production builds always inject a real provisioner.
func HandleCryptoFinalizerOnDelete(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	kp VolumeKeyProvisioner,
	volumeID string,
) (bool, error) {
	if obj.GetDeletionTimestamp().IsZero() {
		return false, nil
	}
	fin := obj.GetFinalizers()
	if !slices.Contains(fin, CryptoFinalizerName) {
		return false, nil
	}
	if kp != nil {
		if err := kp.DestroyVolume(ctx, volumeID); err != nil {
			return true, err
		}
	}
	if _, err := RemoveCryptoFinalizer(ctx, c, obj); err != nil {
		return true, err
	}
	return true, nil
}
