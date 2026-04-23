package controllers

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// KmsKeyReconciler reconciles a KmsKey object. Each KmsKey names a master
// key held in OpenBao Transit; on create the reconciler calls the injected
// VolumeKeyProvisioner to ensure the key exists, and records a Ready
// condition once provisioned. DestroyVolume is invoked on deletion to
// trigger cryptographic erase when supported.
type KmsKeyReconciler struct {
	reconciler.BaseReconciler
	KeyProvisioner reconciler.VolumeKeyProvisioner
	Recorder       record.EventRecorder
}

// Reconcile ensures the named KMS key exists.
func (r *KmsKeyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "KmsKey", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.KmsKey
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	kp := r.KeyProvisioner
	if kp == nil {
		logger.Error(errNoKeyProvisioner, "KmsKey: refusing to reconcile without KeyProvisioner")
		result = "error"
		return ctrl.Result{RequeueAfter: 30 * time.Second}, errNoKeyProvisioner
	}

	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("KmsKey deleting")
		if err := kp.DestroyVolume(ctx, obj.Name); err != nil {
			logger.Error(err, "destroy key failed; leaving wrapped DKs intact")
		}
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "kms key destroyed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerKmsKey); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerKmsKey); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Provision idempotent by name.
	if _, _, err := kp.ProvisionVolume(ctx, obj.Name); err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconcileFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: defaultRequeue}, err
	}
	obj.Status.Phase = "Active"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "key provisioned")
	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "kms key ready")
	return ctrl.Result{RequeueAfter: defaultRequeue}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *KmsKeyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "KmsKey"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "kmskey-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.KmsKey{}).
		Named("KmsKey").
		Complete(r)
}
