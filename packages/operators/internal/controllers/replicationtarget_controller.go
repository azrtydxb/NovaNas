package controllers

import (
	"context"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// ReplicationTargetReconciler reconciles a ReplicationTarget object.
//
// TODO(wave-4): implement real logic. This currently logs and exits so that
// the manager can be wired end-to-end against a real K8s API server without
// touching any downstream subsystems (chunk engine, Keycloak, OpenBao, etc).
type ReplicationTargetReconciler struct {
	reconciler.BaseReconciler
}

// Reconcile is the no-op reconciler entry point.
func (r *ReplicationTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "ReplicationTarget", "key", req.NamespacedName)
	logger.V(1).Info("reconciling ReplicationTarget (no-op)")
	defer r.ObserveReconcile(start, "noop")
	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *ReplicationTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "ReplicationTarget"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.ReplicationTarget{}).
		Named("ReplicationTarget").
		Complete(r)
}
