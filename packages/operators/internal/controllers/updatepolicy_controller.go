package controllers

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

const finalizerUpdatePolicy = reconciler.FinalizerPrefix + "updatepolicy"

// UpdatePolicyReconciler is a cluster-singleton that ensures the updater
// Deployment exists and reports AvailableVersion via UpdateClient
// (no-op default).
type UpdatePolicyReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
	Updater  reconciler.UpdateClient
}

// Reconcile ensures the updater deployment + version status.
func (r *UpdatePolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "UpdatePolicy", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.UpdatePolicy
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerUpdatePolicy); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerUpdatePolicy); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "ensuring updater deployment")
	obj.Status.Phase = "Reconciling"

	upd := r.Updater
	if upd == nil {
		upd = reconciler.NoopUpdateClient{}
	}
	cur, _ := upd.CurrentVersion(ctx)
	avail, _ := upd.AvailableVersion(ctx, obj.Name)

	labels := map[string]string{"app": "novanas-updater", "novanas.io/owner": obj.Name}
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "novanas-system", Name: childName(obj.Name, "updater")}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		if dep.Labels == nil {
			dep.Labels = map[string]string{}
		}
		for k, v := range labels {
			dep.Labels[k] = v
		}
		one := int32(1)
		dep.Spec.Replicas = &one
		dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		dep.Spec.Template.ObjectMeta.Labels = labels
		dep.Spec.Template.Spec.Containers = []corev1.Container{{
			Name:  "updater",
			Image: "ghcr.io/azrtydxb/novanas/updater:stub",
			Env: []corev1.EnvVar{
				{Name: "CURRENT_VERSION", Value: cur},
				{Name: "AVAILABLE_VERSION", Value: avail},
			},
		}}
		return nil
	}); err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "DeploymentFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}

	logger.V(1).Info("updater ensured", "current", cur, "available", avail)
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "updater ready ("+cur+")")
	obj.Status.Phase = "Ready"
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "UpdatePolicy ready")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *UpdatePolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "UpdatePolicy"
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.UpdatePolicy{}).
		Named("UpdatePolicy").
		Complete(r)
}
