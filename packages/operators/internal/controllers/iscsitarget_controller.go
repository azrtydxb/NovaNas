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

// IscsiTargetReconciler reconciles a IscsiTarget object. Actual LIO
// portal wiring is SPDK-gated and out of scope for this wave; the
// reconciler ensures CRD lifecycle (finalizer + Ready condition) and
// emits a warning event reminding operators that the data-plane wire-up
// is pending.
type IscsiTargetReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures finalizer + Ready for IscsiTarget.
func (r *IscsiTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "IscsiTarget", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.IscsiTarget
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("IscsiTarget deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "iscsi target removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerIscsiTarget); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerIscsiTarget); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	logger.Info("iSCSI data-plane wiring is SPDK-gated; CRD ready but no LIO portal configured")
	reconciler.EmitWarning(r.Recorder, &obj, reconciler.EventReasonExternalSync, "iSCSI data-plane wiring pending (SPDK)")
	obj.Status.Phase = "Ready"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "CRD registered; data-plane gated")
	obj.Status.Conditions = reconciler.MarkDegraded(obj.Status.Conditions, obj.Generation, reconciler.ReasonAwaitingExternal, "LIO portal not configured")
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
func (r *IscsiTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "IscsiTarget"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "iscsitarget-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.IscsiTarget{}).
		Named("IscsiTarget").
		Complete(r)
}
