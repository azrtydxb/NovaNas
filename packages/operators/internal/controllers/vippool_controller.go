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

const finalizerVipPool = reconciler.FinalizerPrefix + "vippool"

// VipPoolReconciler projects the pool into a novaedge IPAddressPool CR.
// Falls back to status-only reconcile when the novaedge CRD is absent.
type VipPoolReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the projected novaedge pool exists.
func (r *VipPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "VipPool", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.VipPool
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerVipPool); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerVipPool); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "projecting VIP pool")
	obj.Status.Phase = "Reconciling"

	gvk := schema.GroupVersionKind{Group: "novaedge.io", Version: "v1alpha1", Kind: "IPAddressPool"}
	err := ensureUnstructured(ctx, r.Client, gvk, "novanas-system", childName(obj.Name, "vippool"), func(u *unstructuredType) {
		setSpec(u, map[string]interface{}{
			"owner": obj.Name,
		})
		if u.GetLabels() == nil {
			u.SetLabels(map[string]string{})
		}
		lbl := u.GetLabels()
		lbl["novanas.io/kind"] = "VipPool"
		u.SetLabels(lbl)
	})
	switch {
	case err == nil:
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "VIP pool projected")
		obj.Status.Phase = "Ready"
	case err == errKindMissing:
		logger.V(1).Info("novaedge IPAddressPool CRD not installed -- status only")
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonAwaitingExternal, "novaedge CRD absent; status-only")
		obj.Status.Phase = "Pending"
	default:
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "ProjectionFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "VipPool reconciled")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *VipPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "VipPool"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.VipPool{}).
		Named("VipPool").
		Complete(r)
}
