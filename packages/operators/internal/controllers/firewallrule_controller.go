package controllers

import (
	"context"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// FirewallRuleReconciler reconciles a FirewallRule object.
//
// TODO(wave-4): implement real logic. This currently logs and exits so that
// the manager can be wired end-to-end against a real K8s API server without
// touching any downstream subsystems (chunk engine, Keycloak, OpenBao, etc).
type FirewallRuleReconciler struct {
	reconciler.BaseReconciler
}

// Reconcile is the no-op reconciler entry point.
func (r *FirewallRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "FirewallRule", "key", req.NamespacedName)
	logger.V(1).Info("reconciling FirewallRule (no-op)")
	defer r.ObserveReconcile(start, "noop")
	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *FirewallRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "FirewallRule"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.FirewallRule{}).
		Named("FirewallRule").
		Complete(r)
}
