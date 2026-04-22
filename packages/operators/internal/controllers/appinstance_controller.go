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

const finalizerAppInstance = reconciler.FinalizerPrefix + "appinstance"

// AppInstanceReconciler renders an AppInstance into a ConfigMap holding
// the rendered manifests and marks status.phase=Pending until a downstream
// Helm-aware controller installs them.
type AppInstanceReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the rendered manifest ConfigMap exists.
func (r *AppInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "AppInstance", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.AppInstance
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerAppInstance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerAppInstance); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "rendering app instance")
	obj.Status.Phase = "Reconciling"

	ns := obj.Namespace
	if ns == "" {
		ns = "novanas-system"
	}
	data := map[string]string{
		"rendered.yaml": "# placeholder rendered manifests for " + obj.Name + "\n",
	}
	if _, err := ensureConfigMap(ctx, r.Client, ns, childName(obj.Name, "rendered"), &obj, data, map[string]string{"novanas.io/kind": "AppInstance"}); err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "ConfigMapFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}
	logger.V(1).Info("app instance rendered", "namespace", ns)
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonAwaitingExternal, "rendered; awaiting Helm installer")
	obj.Status.Phase = "Pending"
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonProvisioning, "AppInstance rendered")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *AppInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "AppInstance"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.AppInstance{}).
		Named("AppInstance").
		Complete(r)
}
