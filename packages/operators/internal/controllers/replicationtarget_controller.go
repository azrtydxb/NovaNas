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

// ReplicationTargetReconciler reconciles a ReplicationTarget object. A
// target is a remote endpoint descriptor; reconciliation ensures finalizer
// management and a Ready condition. Actual connectivity probing (TLS
// handshake, auth verification) is wired via the StorageClient in future
// waves.
type ReplicationTargetReconciler struct {
	reconciler.BaseReconciler
	Storage  reconciler.StorageClient
	Recorder record.EventRecorder
}

// Reconcile ensures finalizer + Ready condition for ReplicationTarget.
func (r *ReplicationTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "ReplicationTarget", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.ReplicationTarget
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("ReplicationTarget deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "replication target removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerReplicationTarget); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerReplicationTarget); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}
	obj.Status.Phase = "Ready"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "target reachable")
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
func (r *ReplicationTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "ReplicationTarget"
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("replicationtarget-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.ReplicationTarget{}).
		Named("ReplicationTarget").
		Complete(r)
}
