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

// BucketReconciler reconciles a Bucket object.
//
// Wave 6 scope: key-provisioning on create when spec.encryption.enabled.
// S3 object-store bucket creation + user/policy wiring remains TODO.
type BucketReconciler struct {
	reconciler.BaseReconciler
	KeyProvisioner reconciler.VolumeKeyProvisioner
}

// Reconcile wires encryption provisioning for Bucket.
func (r *BucketReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "Bucket", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var b novanasv1alpha1.Bucket
	if err := r.Client.Get(ctx, req.NamespacedName, &b); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if b.Spec.Encryption == nil || !b.Spec.Encryption.Enabled {
		return ctrl.Result{}, nil
	}

	kp := r.KeyProvisioner
	if kp == nil {
		logger.Error(errNoKeyProvisioner, "Bucket: refusing to reconcile encrypted bucket without KeyProvisioner")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, errNoKeyProvisioner
	}

	volumeID := string(b.UID)
	if volumeID == "" {
		volumeID = b.Name
	}

	if handled, err := reconciler.HandleCryptoFinalizerOnDelete(ctx, r.Client, &b, kp, volumeID); handled || err != nil {
		return ctrl.Result{}, err
	}
	if _, err := reconciler.EnsureCryptoFinalizer(ctx, r.Client, &b); err != nil {
		return ctrl.Result{}, err
	}

	if b.Status.Encryption != nil && b.Status.Encryption.Provisioned {
		return ctrl.Result{}, nil
	}

	wrapped, version, err := kp.ProvisionVolume(ctx, volumeID)
	if err != nil {
		logger.Error(err, "ProvisionVolume failed")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	now := metav1.NewTime(time.Now())
	b.Status.Encryption = &novanasv1alpha1.EncryptionStatus{
		Provisioned:   true,
		WrappedDK:     wrapped,
		KeyVersion:    version,
		ProvisionedAt: &now,
	}
	if err := r.Client.Status().Update(ctx, &b); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *BucketReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "Bucket"
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.Bucket{}).
		Named("Bucket").
		Complete(r)
}
