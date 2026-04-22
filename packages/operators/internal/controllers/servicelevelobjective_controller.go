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

const finalizerServiceLevelObjective = reconciler.FinalizerPrefix + "servicelevelobjective"

// ServiceLevelObjectiveReconciler projects the SLO into a PrometheusRule
// containing burn-rate expressions. Falls back to status-only when the
// CRD is absent.
type ServiceLevelObjectiveReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the SLO PrometheusRule.
func (r *ServiceLevelObjectiveReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "ServiceLevelObjective", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.ServiceLevelObjective
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerServiceLevelObjective); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerServiceLevelObjective); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "projecting SLO burn-rate rules")
	obj.Status.Phase = "Reconciling"

	gvk := schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "PrometheusRule"}
	err := ensureUnstructured(ctx, r.Client, gvk, "novanas-system", childName(obj.Name, "slo"), func(u *unstructuredType) {
		setSpec(u, map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{
					"name": obj.Name + "-burnrate",
					"rules": []interface{}{
						map[string]interface{}{
							"record": obj.Name + ":error_budget_burn",
							"expr":   "rate(errors_total[5m]) / rate(requests_total[5m])",
						},
					},
				},
			},
		})
	})
	switch err {
	case nil:
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "SLO rules projected")
		obj.Status.Phase = "Ready"
	case errKindMissing:
		logger.V(1).Info("PrometheusRule CRD absent -- status only")
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonAwaitingExternal, "prometheus-operator absent; status-only")
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
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "ServiceLevelObjective reconciled")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *ServiceLevelObjectiveReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "ServiceLevelObjective"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.ServiceLevelObjective{}).
		Named("ServiceLevelObjective").
		Complete(r)
}
