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

const finalizerFirewallRule = reconciler.FinalizerPrefix + "firewallrule"

// FirewallRuleReconciler projects the rule into a novanet FirewallRule CR.
// Falls back to a ConfigMap snapshot when the novanet CRD is absent so the
// host agent can still pick up the rendered ruleset.
type FirewallRuleReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the projected firewall rule exists.
func (r *FirewallRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "FirewallRule", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.FirewallRule
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerFirewallRule); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerFirewallRule); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "projecting firewall rule")
	obj.Status.Phase = "Reconciling"

	rendered := renderFirewallRule(&obj)
	h := hashBytes([]byte(rendered))

	// Always mirror to a ConfigMap so the host agent has a canonical copy.
	_, _ = ensureConfigMap(ctx, r.Client, "novanas-system", childName(obj.Name, "fw"), &obj, map[string]string{
		"rule.nft": rendered,
		"hash":     h,
	}, map[string]string{"novanas.io/kind": "FirewallRule"})

	gvk := schema.GroupVersionKind{Group: "novanet.io", Version: "v1alpha1", Kind: "FirewallRule"}
	err := ensureUnstructured(ctx, r.Client, gvk, "novanas-system", childName(obj.Name, "fw"), func(u *unstructuredType) {
		setSpec(u, map[string]interface{}{
			"owner":     obj.Name,
			"scope":     string(obj.Spec.Scope),
			"direction": string(obj.Spec.Direction),
			"action":    string(obj.Spec.Action),
			"hash":      h,
		})
	})
	switch err {
	case nil:
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "firewall rule projected")
		obj.Status.Phase = "Active"
	case errKindMissing:
		logger.V(1).Info("novanet FirewallRule CRD absent -- ConfigMap-only reconcile")
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonAwaitingExternal, "novanet CRD absent; rule staged in ConfigMap")
		obj.Status.Phase = "Pending"
	default:
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "ProjectionFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}
	now := metav1.Now()
	obj.Status.InstalledAt = &now
	obj.Status.AppliedConfigHash = h
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "FirewallRule reconciled")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *FirewallRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "FirewallRule"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.FirewallRule{}).
		Named("FirewallRule").
		Complete(r)
}
