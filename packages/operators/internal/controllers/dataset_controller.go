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

// DatasetReconciler reconciles a Dataset object.
//
// Wave 6 scope: key-provisioning on create when spec.encryption.enabled.
// Filesystem creation, mount, and quota enforcement remain TODO.
type DatasetReconciler struct {
	reconciler.BaseReconciler
	// KeyProvisioner is injected at wire-up time. If nil a noop is used.
	KeyProvisioner reconciler.VolumeKeyProvisioner
}

// Reconcile wires the encryption provisioning path for Dataset.
func (r *DatasetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "Dataset", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var ds novanasv1alpha1.Dataset
	if err := r.Client.Get(ctx, req.NamespacedName, &ds); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if ds.Spec.Encryption == nil || !ds.Spec.Encryption.Enabled {
		return ctrl.Result{}, nil
	}
	if ds.Status.Encryption != nil && ds.Status.Encryption.Provisioned {
		return ctrl.Result{}, nil
	}

	kp := r.KeyProvisioner
	if kp == nil {
		logger.Info("Dataset: no KeyProvisioner wired -- using noop (dev only)")
		kp = reconciler.NoopKeyProvisioner{}
	}

	volumeID := string(ds.UID)
	if volumeID == "" {
		volumeID = ds.Name
	}

	wrapped, version, err := kp.ProvisionVolume(ctx, volumeID)
	if err != nil {
		logger.Error(err, "ProvisionVolume failed")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	now := metav1.NewTime(time.Now())
	ds.Status.Encryption = &novanasv1alpha1.EncryptionStatus{
		Provisioned:   true,
		WrappedDK:     wrapped,
		KeyVersion:    version,
		ProvisionedAt: &now,
	}
	if err := r.Client.Status().Update(ctx, &ds); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *DatasetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "Dataset"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.Dataset{}).
		Named("Dataset").
		Complete(r)
}
