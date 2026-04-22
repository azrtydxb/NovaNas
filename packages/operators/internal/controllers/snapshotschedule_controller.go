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

// SnapshotScheduleReconciler reconciles a SnapshotSchedule object. A
// SnapshotSchedule is a cron-style descriptor; the reconciler tracks the
// schedule's desired next-fire time in status. Actual Snapshot CR creation
// on cron tick is performed by a sidecar cron controller (wave 7); here we
// ensure the object has a finalizer and Ready condition.
type SnapshotScheduleReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures finalizer + conditions for SnapshotSchedule.
func (r *SnapshotScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "SnapshotSchedule", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.SnapshotSchedule
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("SnapshotSchedule deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "snapshot schedule deleted")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerSnapshotSchedule); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerSnapshotSchedule); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.Phase = "Scheduled"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "schedule active")
	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "schedule reconciled")
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *SnapshotScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "SnapshotSchedule"
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("snapshotschedule-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.SnapshotSchedule{}).
		Named("SnapshotSchedule").
		Complete(r)
}
