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

const finalizerServicePolicy = reconciler.FinalizerPrefix + "servicepolicy"

// ServicePolicyReconciler publishes service enable/disable knobs into a
// ConfigMap; a downstream sidecar patches the target Deployment's
// replica count. We intentionally keep this reconciler out of the direct
// Deployment edit path so multiple policies can compose without races.
type ServicePolicyReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the service-policy ConfigMap.
func (r *ServicePolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "ServicePolicy", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.ServicePolicy
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerServicePolicy); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerServicePolicy); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "publishing service policy")
	obj.Status.Phase = "Reconciling"

	if _, err := ensureConfigMap(ctx, r.Client, "novanas-system", childName(obj.Name, "service-policy"), &obj, map[string]string{"policy": obj.Name, "enabled": "true"}, map[string]string{"novanas.io/kind": "ServicePolicy"}); err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "ConfigMapFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}
	logger.V(1).Info("service policy published")
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "service policy ready")
	obj.Status.Phase = "Ready"
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "ServicePolicy ready")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *ServicePolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "ServicePolicy"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.ServicePolicy{}).
		Named("ServicePolicy").
		Complete(r)
}
