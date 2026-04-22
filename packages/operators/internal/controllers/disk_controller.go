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

// DiskReconciler reconciles a Disk object.
//
// The reconciler drives the Disk state machine: if no state has been set the
// disk is marked Identified; once assigned to a pool and observed healthy it
// is promoted to Active; failed SMART readings flip it to Degraded/Failed.
// Actual block-device discovery is performed by the on-node hardware agent
// which writes spec/status fields the operator interprets.
type DiskReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile advances the Disk state machine.
func (r *DiskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "Disk", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var d novanasv1alpha1.Disk
	if err := r.Client.Get(ctx, req.NamespacedName, &d); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !d.DeletionTimestamp.IsZero() {
		logger.Info("Disk deleting")
		reconciler.Emit(r.Recorder, &d, reconciler.EventReasonDeleted, "disk removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &d, reconciler.FinalizerDisk); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &d, reconciler.FinalizerDisk); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	prev := d.Status.State
	switch {
	case d.Status.State == "":
		d.Status.State = novanasv1alpha1.DiskStateIdentified
	case d.Status.Smart != nil && d.Status.Smart.OverallHealth == "FAILED":
		d.Status.State = novanasv1alpha1.DiskStateFailed
	case d.Status.Smart != nil && d.Status.Smart.OverallHealth == "WARNING":
		d.Status.State = novanasv1alpha1.DiskStateDegraded
	case d.Spec.Pool != "" && d.Status.State == novanasv1alpha1.DiskStateIdentified:
		d.Status.State = novanasv1alpha1.DiskStateAssigned
	case d.Status.State == novanasv1alpha1.DiskStateAssigned:
		d.Status.State = novanasv1alpha1.DiskStateActive
	}
	if prev != d.Status.State {
		reconciler.Emit(r.Recorder, &d, reconciler.EventReasonUpdated,
			"disk state: "+string(prev)+" -> "+string(d.Status.State))
	}

	msg := "disk state=" + string(d.Status.State)
	switch d.Status.State {
	case novanasv1alpha1.DiskStateActive:
		d.Status.Conditions = reconciler.MarkReady(d.Status.Conditions, d.Generation, reconciler.ReasonReconciled, msg)
	case novanasv1alpha1.DiskStateFailed:
		d.Status.Conditions = reconciler.MarkFailed(d.Status.Conditions, d.Generation, "SmartFailed", msg)
	case novanasv1alpha1.DiskStateDegraded:
		d.Status.Conditions = reconciler.MarkDegraded(d.Status.Conditions, d.Generation, "SmartWarning", msg)
	default:
		d.Status.Conditions = reconciler.MarkProgressing(d.Status.Conditions, d.Generation, reconciler.ReasonReconciling, msg)
	}

	if err := r.Client.Status().Update(ctx, &d); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: defaultRequeue}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *DiskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "Disk"
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("disk-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.Disk{}).
		Named("Disk").
		Complete(r)
}
