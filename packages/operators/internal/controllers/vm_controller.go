package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

const finalizerVm = reconciler.FinalizerPrefix + "vm"

// VmReconciler projects a NovaNas Vm into a kubevirt.io/v1
// VirtualMachine. When KubeVirt is absent (CRD missing or VmEngine is
// NoopVmEngine) the reconciler records Phase=Pending and exits clean.
//
// In addition to desired-state reconcile, the reconciler consumes E1's
// action surface:
//   - spec.powerState (Running/Stopped/Paused) — drives VmEngine.SetPowerState
//   - annotation novanas.io/action-reset — triggers a hard reset
type VmReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
	Engine   reconciler.VmEngine
}

// Reconcile ensures the backing KubeVirt VM exists.
func (r *VmReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "Vm", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.Vm
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		eng := r.Engine
		if eng != nil {
			_ = eng.DeleteVM(ctx, obj.Namespace, obj.Name)
		}
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerVm); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerVm); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	eng := r.Engine
	if eng == nil {
		eng = reconciler.NoopVmEngine{}
	}

	// --- Action-reset annotation ------------------------------------
	if _, err := reconciler.HandleActionAnnotation(ctx, r.Client, &obj, "reset",
		func(ctx context.Context, _ client.Object) error {
			logger.Info("action-reset: restarting VM")
			reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonProvisioning, "VM reset requested")
			return eng.Restart(ctx, obj.Namespace, obj.Name)
		}); err != nil {
		logger.Error(err, "reset handler failed")
	}

	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "projecting VM")
	obj.Status.Phase = "Reconciling"

	phase, err := eng.EnsureVM(ctx, obj.Namespace, obj.Name, map[string]interface{}{"owner": obj.Name})
	if err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "EngineFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// --- Power-state reconciliation ---------------------------------
	if desired := obj.Spec.PowerState; desired != "" {
		if perr := eng.SetPowerState(ctx, obj.Namespace, obj.Name, desired); perr != nil {
			logger.V(1).Info("set power state failed", "state", desired, "error", perr.Error())
		} else {
			phase = desired
		}
	}

	// Also project into a KubeVirt VirtualMachine when available. Best-effort
	// so the VmEngine remains the primary authority.
	gvk := schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachine"}
	ns := obj.Namespace
	if ns == "" {
		ns = "novanas-system"
	}
	if pErr := ensureUnstructured(ctx, r.Client, gvk, ns, obj.Name, func(u *unstructuredType) {
		setSpec(u, map[string]interface{}{
			"running": true,
		})
	}); pErr != nil && pErr != errKindMissing {
		logger.V(1).Info("kubevirt projection failed", "error", pErr.Error())
	}

	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "VM projected: "+phase)
	obj.Status.Phase = phase
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "Vm "+phase)
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *VmReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "Vm"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.Vm{}).
		Named("Vm").
		Complete(r)
}
