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

// ScrubScheduleReconciler reconciles a ScrubSchedule object. The schedule
// triggers StorageClient.StartScrub periodically; for now the reconciler
// manages lifecycle state (finalizer, Ready). Cron firing is future work.
type ScrubScheduleReconciler struct {
	reconciler.BaseReconciler
	Storage  reconciler.StorageClient
	Recorder record.EventRecorder
}

// Reconcile ensures finalizer + Ready for ScrubSchedule.
func (r *ScrubScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "ScrubSchedule", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.ScrubSchedule
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("ScrubSchedule deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "scrub schedule removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerScrubSchedule); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerScrubSchedule); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}
	obj.Status.Phase = "Scheduled"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "scrub schedule active")
	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *ScrubScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "ScrubSchedule"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "scrubschedule-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.ScrubSchedule{}).
		Named("ScrubSchedule").
		Complete(r)
}
