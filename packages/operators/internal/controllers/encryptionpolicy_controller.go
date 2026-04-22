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

// EncryptionPolicyReconciler reconciles a cluster-scope EncryptionPolicy.
// The policy describes which volumes should be encrypted and which KMS key
// version to use. This reconciler manages lifecycle state; policy
// enforcement happens in BlockVolume/Dataset/Bucket reconcilers that
// consult the policy when provisioning.
type EncryptionPolicyReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures finalizer + Ready for EncryptionPolicy.
func (r *EncryptionPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "EncryptionPolicy", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.EncryptionPolicy
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("EncryptionPolicy deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "encryption policy removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerEncryptionPolicy); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerEncryptionPolicy); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}
	obj.Status.Phase = "Active"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "policy active")
	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: defaultRequeue}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *EncryptionPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "EncryptionPolicy"
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("encryptionpolicy-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.EncryptionPolicy{}).
		Named("EncryptionPolicy").
		Complete(r)
}
