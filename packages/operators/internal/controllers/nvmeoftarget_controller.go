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

// NvmeofTargetReconciler reconciles a NvmeofTarget object. Actual
// nvmet/SPDK subsystem wiring is out of scope for this wave; the
// reconciler ensures CRD lifecycle and reports Degraded while the
// data-plane wire-up is pending.
type NvmeofTargetReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures finalizer + Ready for NvmeofTarget.
func (r *NvmeofTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "NvmeofTarget", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.NvmeofTarget
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("NvmeofTarget deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "nvme-of target removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerNvmeofTarget); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerNvmeofTarget); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	logger.Info("NVMe-oF data-plane wiring is SPDK-gated; CRD ready but no subsystem configured")
	reconciler.EmitWarning(r.Recorder, &obj, reconciler.EventReasonExternalSync, "nvme-of data-plane wiring pending (SPDK)")
	obj.Status.Phase = "Ready"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "CRD registered; data-plane gated")
	obj.Status.Conditions = reconciler.MarkDegraded(obj.Status.Conditions, obj.Generation, reconciler.ReasonAwaitingExternal, "SPDK subsystem not configured")
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
func (r *NvmeofTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "NvmeofTarget"
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "nvmeoftarget-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.NvmeofTarget{}).
		Named("NvmeofTarget").
		Complete(r)
}
