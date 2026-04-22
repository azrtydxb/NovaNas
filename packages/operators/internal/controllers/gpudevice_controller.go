package controllers

import (
	"context"
	"time"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

const finalizerGpuDevice = reconciler.FinalizerPrefix + "gpudevice"

// GpuDeviceReconciler is a status-only observer of a GPU device.
// Future work: project into a kubevirt passthrough device plugin CR.
type GpuDeviceReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile observes the GPU device.
func (r *GpuDeviceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "GpuDevice", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.GpuDevice
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerGpuDevice); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerGpuDevice); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	logger.V(1).Info("gpu device observed")
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "gpu observed")
	obj.Status.Phase = "Observed"
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "GpuDevice observed")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *GpuDeviceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "GpuDevice"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.GpuDevice{}).
		Named("GpuDevice").
		Complete(r)
}
