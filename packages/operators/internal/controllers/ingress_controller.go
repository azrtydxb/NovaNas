package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

const finalizerIngress = reconciler.FinalizerPrefix + "ingress"

// IngressReconciler projects a NovaNas Ingress into a novaedge Route.
// Falls back to a ConfigMap snapshot when the novaedge CRD is absent.
type IngressReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the projected novaedge Route exists.
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "Ingress", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.Ingress
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerIngress); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerIngress); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "projecting ingress")
	obj.Status.Phase = "Reconciling"

	// Render a stable ruleset string for hashing/audit.
	var rules []string
	for _, rule := range obj.Spec.Rules {
		rules = append(rules, fmt.Sprintf("%s%s -> %s", rule.Host, rule.Path, rule.Backend))
	}
	rendered := fmt.Sprintf("hostname: %s\nrules:\n  - %s\n", obj.Spec.Hostname, strings.Join(rules, "\n  - "))
	if obj.Spec.Tls != nil {
		rendered += "tls: " + obj.Spec.Tls.Certificate + "\n"
	}
	h := hashBytes([]byte(rendered))

	ns := childNamespace(&obj)
	_, _ = ensureConfigMap(ctx, r.Client, ns, childName(obj.Name, "ingress-snapshot"), &obj, map[string]string{
		"route.yaml": rendered,
		"hash":       h,
	}, map[string]string{"novanas.io/kind": "Ingress"})

	gvk := schema.GroupVersionKind{Group: "novaedge.io", Version: "v1alpha1", Kind: "Route"}
	err := ensureUnstructured(ctx, r.Client, gvk, ns, childName(obj.Name, "ingress"), func(u *unstructuredType) {
		setSpec(u, map[string]interface{}{
			"owner":    obj.Name,
			"hostname": obj.Spec.Hostname,
			"hash":     h,
		})
	})
	switch err {
	case nil:
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "route projected")
		obj.Status.Phase = "Active"
		// The novaedge controller is responsible for assigning the VIP;
		// populate a placeholder until the observed Route reports it.
		if obj.Status.Vip == "" {
			obj.Status.Vip = "pending-allocation"
		}
	case errKindMissing:
		logger.V(1).Info("novaedge Route CRD absent -- ConfigMap-only reconcile")
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonAwaitingExternal, "novaedge CRD absent; route staged in ConfigMap")
		obj.Status.Phase = "Pending"
	default:
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "ProjectionFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}
	obj.Status.AppliedConfigHash = h
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "Ingress reconciled")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "Ingress"
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.Ingress{}).
		Named("Ingress").
		Complete(r)
}
