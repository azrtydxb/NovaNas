package controllers

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// BlockVolumeReconciler reconciles a BlockVolume object.
//
// Wave 6 scope: if spec.encryption.enabled is true and the volume has not
// yet been key-provisioned, call VolumeKeyProvisioner.ProvisionVolume to
// generate + wrap a Dataset Key and persist it in status.encryption. All
// other lifecycle logic (chunk engine provisioning, NVMe-oF target
// creation, etc.) remains TODO for later waves.
type BlockVolumeReconciler struct {
	reconciler.BaseReconciler
	// KeyProvisioner is injected at wire-up time. If nil the controller
	// uses a no-op provisioner and logs a warning.
	KeyProvisioner reconciler.VolumeKeyProvisioner
}

// Reconcile wires the encryption path for BlockVolume.
func (r *BlockVolumeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "BlockVolume", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var bv novanasv1alpha1.BlockVolume
	if err := r.Client.Get(ctx, req.NamespacedName, &bv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Encryption provisioning is idempotent: skip if already provisioned
	// or if encryption is not requested.
	if bv.Spec.Encryption == nil || !bv.Spec.Encryption.Enabled {
		logger.V(1).Info("BlockVolume unencrypted, skipping key provisioning")
		return ctrl.Result{}, nil
	}

	kp := r.KeyProvisioner
	if kp == nil {
		// Encryption is a security-critical code path. Refuse to fall back
		// to a noop provisioner in production — a silent placeholder wrap
		// would render the volume unrecoverable later. The operator MUST be
		// constructed with a real VolumeKeyProvisioner (TransitKeyProvisioner
		// or an in-process VolumeKeyManager shim).
		err := errNoKeyProvisioner
		logger.Error(err, "BlockVolume: refusing to reconcile encrypted volume without KeyProvisioner")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	volumeID := string(bv.UID)
	if volumeID == "" {
		volumeID = bv.Name
	}

	// Crypto-finalizer: DestroyVolume on delete, then drop finalizer.
	if handled, err := reconciler.HandleCryptoFinalizerOnDelete(ctx, r.Client, &bv, kp, volumeID); handled || err != nil {
		return ctrl.Result{}, err
	}
	if _, err := reconciler.EnsureCryptoFinalizer(ctx, r.Client, &bv); err != nil {
		return ctrl.Result{}, err
	}

	if bv.Status.Encryption != nil && bv.Status.Encryption.Provisioned {
		return ctrl.Result{}, nil
	}

	wrapped, version, err := kp.ProvisionVolume(ctx, volumeID)
	if err != nil {
		logger.Error(err, "ProvisionVolume failed")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	now := metav1.NewTime(time.Now())
	bv.Status.Encryption = &novanasv1alpha1.EncryptionStatus{
		Provisioned:   true,
		WrappedDK:     wrapped,
		KeyVersion:    version,
		ProvisionedAt: &now,
	}
	if bv.Status.Phase == "" {
		bv.Status.Phase = "Encrypted"
	}
	if err := r.Client.Status().Update(ctx, &bv); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *BlockVolumeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "BlockVolume"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.BlockVolume{}).
		Named("BlockVolume").
		Complete(r)
}
