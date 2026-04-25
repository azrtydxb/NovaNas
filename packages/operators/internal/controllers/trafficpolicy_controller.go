package controllers

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

const finalizerTrafficPolicy = reconciler.FinalizerPrefix + "trafficpolicy"

// TrafficPolicyReconciler projects the policy into a novanet
// TrafficPolicy CR. Falls back to a ConfigMap snapshot when the CRD is
// absent so the host agent can still pick up the limiter config.
type TrafficPolicyReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the projected traffic policy exists.
func (r *TrafficPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "TrafficPolicy", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.TrafficPolicy
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerTrafficPolicy); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerTrafficPolicy); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "projecting traffic policy")
	obj.Status.Phase = "Reconciling"

	rendered := renderTrafficLimits(&obj)
	h := hashBytes([]byte(rendered))
	_, _ = ensureConfigMap(ctx, r.Client, "novanas-system", childName(obj.Name, "tp"), &obj, map[string]string{
		"config": rendered,
		"hash":   h,
	}, map[string]string{"novanas.io/kind": "TrafficPolicy"})

	gvk := schema.GroupVersionKind{Group: "novanet.io", Version: "v1alpha1", Kind: "TrafficPolicy"}
	err := ensureUnstructured(ctx, r.Client, gvk, "novanas-system", childName(obj.Name, "tp"), func(u *unstructuredType) {
		setSpec(u, map[string]interface{}{
			"owner":     obj.Name,
			"scopeKind": string(obj.Spec.Scope.Kind),
			"scopeName": obj.Spec.Scope.Name,
			"hash":      h,
		})
	})
	switch err {
	case nil:
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "traffic policy projected")
		obj.Status.Phase = "Active"
	case errKindMissing:
		logger.V(1).Info("novanet TrafficPolicy CRD absent -- ConfigMap-only reconcile")
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonAwaitingExternal, "novanet CRD absent; policy staged in ConfigMap")
		obj.Status.Phase = "Pending"
	default:
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "ProjectionFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}
	now := metav1.Now()
	obj.Status.AppliedAt = &now
	obj.Status.AppliedConfigHash = h
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "TrafficPolicy reconciled")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *TrafficPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "TrafficPolicy"
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.TrafficPolicy{}).
		Named("TrafficPolicy").
		Complete(r)
}
