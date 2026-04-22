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

const finalizerCustomDomain = reconciler.FinalizerPrefix + "customdomain"

// CustomDomainReconciler projects the domain into a novaedge
// HostnameBinding CR. Falls back to status-only when absent.
type CustomDomainReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the projected hostname binding.
func (r *CustomDomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "CustomDomain", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.CustomDomain
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerCustomDomain); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerCustomDomain); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "projecting custom domain")
	obj.Status.Phase = "Reconciling"

	gvk := schema.GroupVersionKind{Group: "novaedge.io", Version: "v1alpha1", Kind: "HostnameBinding"}
	err := ensureUnstructured(ctx, r.Client, gvk, "novanas-system", childName(obj.Name, "hostname"), func(u *unstructuredType) {
		setSpec(u, map[string]interface{}{"owner": obj.Name})
	})
	switch err {
	case nil:
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "custom domain projected")
		obj.Status.Phase = "Ready"
	case errKindMissing:
		logger.V(1).Info("novaedge HostnameBinding CRD absent -- status only")
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
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "CustomDomain reconciled")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *CustomDomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "CustomDomain"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.CustomDomain{}).
		Named("CustomDomain").
		Complete(r)
}
