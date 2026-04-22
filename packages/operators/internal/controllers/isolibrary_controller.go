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

const finalizerIsoLibrary = reconciler.FinalizerPrefix + "isolibrary"

// IsoLibraryReconciler records desired ISOs into a ConfigMap that a
// downstream downloader Job (future work) consumes. It never pulls bytes
// from the manager process.
type IsoLibraryReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the ISO manifest ConfigMap exists.
func (r *IsoLibraryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "IsoLibrary", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.IsoLibrary
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerIsoLibrary); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerIsoLibrary); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "publishing iso manifest")
	obj.Status.Phase = "Reconciling"

	if _, err := ensureConfigMap(ctx, r.Client, "novanas-system", childName(obj.Name, "iso-manifest"), &obj, map[string]string{"library": obj.Name}, map[string]string{"novanas.io/kind": "IsoLibrary"}); err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "ConfigMapFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}
	logger.V(1).Info("iso manifest written")
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonAwaitingExternal, "manifest published; awaiting downloader")
	obj.Status.Phase = "Pending"
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "IsoLibrary published")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *IsoLibraryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "IsoLibrary"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.IsoLibrary{}).
		Named("IsoLibrary").
		Complete(r)
}
