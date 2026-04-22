package controllers

import (
	"context"
	"time"

	batchv1 "k8s.io/api/batch/v1"
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

const finalizerConfigBackupPolicy = reconciler.FinalizerPrefix + "configbackuppolicy"

// ConfigBackupPolicyReconciler ensures a CronJob running the config-backup
// binary according to the policy schedule.
type ConfigBackupPolicyReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the config-backup CronJob.
func (r *ConfigBackupPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "ConfigBackupPolicy", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.ConfigBackupPolicy
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerConfigBackupPolicy); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerConfigBackupPolicy); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "ensuring config-backup cron")
	obj.Status.Phase = "Reconciling"

	cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Namespace: "novanas-system", Name: childName(obj.Name, "cfgbak")}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cj, func() error {
		if cj.Labels == nil {
			cj.Labels = map[string]string{}
		}
		cj.Labels["novanas.io/owner"] = obj.Name
		cj.Labels["novanas.io/kind"] = "ConfigBackupPolicy"
		cj.Spec.Schedule = "0 2 * * *"
		cj.Spec.JobTemplate.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
		cj.Spec.JobTemplate.Spec.Template.Spec.Containers = []corev1.Container{{
			Name:  "config-backup",
			Image: "ghcr.io/azrtydxb/novanas/config-backup:stub",
		}}
		return nil
	}); err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "CronJobFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}
	logger.V(1).Info("config-backup cron ensured")
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "config-backup cron ready")
	obj.Status.Phase = "Ready"
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "ConfigBackupPolicy ready")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *ConfigBackupPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "ConfigBackupPolicy"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.ConfigBackupPolicy{}).
		Named("ConfigBackupPolicy").
		Complete(r)
}
