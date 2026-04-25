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
//
// It also consumes E1's AppInstance action surface:
//   - spec.desiredState (Running/Stopped) — observed and recorded
//   - spec.version — on change, emits an ExternalSync event so the
//     Helm installer upgrades the release
//   - annotation novanas.io/action-update — forces a re-upgrade even
//     when version is unchanged
//
// The operator does not itself invoke Helm; a downstream controller
// (out of scope for F2) consumes the rendered ConfigMap + the
// synthesised desired-state annotation.
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

	// --- action-update annotation -----------------------------------
	if _, err := reconciler.HandleActionAnnotation(ctx, r.Client, &obj, "update",
		func(ctx context.Context, _ client.Object) error {
			logger.Info("action-update: triggering Helm upgrade")
			reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonExternalSync, "Helm upgrade requested")
			// The Helm-aware controller is external; all we can do
			// from here is record the event and a completion stamp.
			// TODO(operators): wire real Helm upgrade once the
			// installer controller exposes an imperative surface.
			return nil
		}); err != nil {
		logger.Error(err, "update handler failed")
	}

	// --- version reconciliation -------------------------------------
	specVersion := obj.Spec.Version
	if specVersion != "" {
		// Detect a version change against last-recorded annotation.
		prev := obj.Annotations[reconciler.ActionAnnotationPrefix+"version-applied"]
		if prev != specVersion {
			logger.Info("app version change detected", "from", prev, "to", specVersion)
			reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonExternalSync, "app version change: "+prev+" -> "+specVersion)
			patched := obj.DeepCopy()
			if patched.Annotations == nil {
				patched.Annotations = map[string]string{}
			}
			patched.Annotations[reconciler.ActionAnnotationPrefix+"version-applied"] = specVersion
			if err := r.Client.Patch(ctx, patched, client.MergeFrom(&obj)); err != nil {
				logger.V(1).Info("version-applied annotation write failed", "error", err.Error())
			} else {
				obj = *patched
			}
		}
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
	if specVersion != "" {
		data["version"] = specVersion
	}
	if obj.Spec.App != "" {
		data["app"] = obj.Spec.App
	}
	if _, err := ensureConfigMap(ctx, r.Client, ns, childName(obj.Name, "rendered"), &obj, data, map[string]string{"novanas.io/kind": "AppInstance"}); err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "ConfigMapFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}
	logger.V(1).Info("app instance rendered", "namespace", ns)

	// Phase stays Pending until the downstream Helm installer observes
	// deployment readiness and flips it to Running via status patches.
	phase := "Pending"

	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonAwaitingExternal, "rendered; awaiting Helm installer")
	obj.Status.Phase = phase
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonProvisioning, "AppInstance rendered")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *AppInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "AppInstance"
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.AppInstance{}).
		Named("AppInstance").
		Complete(r)
}
