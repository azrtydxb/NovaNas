package controllers

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// SmbServerReconciler reconciles a SmbServer object. It ensures a Samba
// Deployment exists with the requested replica count and propagates the
// Deployment's ReadyReplicas into status.
type SmbServerReconciler struct {
	reconciler.BaseReconciler
	Recorder      record.EventRecorder
	ContainerImage string // injected at wire-up; defaults below when empty
}

// Reconcile ensures the Samba Deployment.
func (r *SmbServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "SmbServer", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.SmbServer
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("SmbServer deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "smb server removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerSmbServer); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerSmbServer); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	ns := obj.Namespace
	if ns == "" {
		ns = "novanas-system"
	}
	img := r.ContainerImage
	if img == "" {
		img = "ghcr.io/azrtydxb/novanas/samba:dev"
	}
	replicas := obj.Spec.Replicas
	if replicas == 0 {
		replicas = 1
	}
	dep := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "smb-" + obj.Name}}
	labels := map[string]string{"app.kubernetes.io/name": "smbserver", "novanas.io/smbserver": obj.Name}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, &dep, func() error {
		dep.Spec.Replicas = &replicas
		dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		dep.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "samba", Image: img,
			}}},
		}
		return controllerutil.SetControllerReference(&obj, &dep, r.Scheme)
	})
	if err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconcileFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonChildEnsured, "samba deployment "+string(op))
	}

	obj.Status.ReadyReplicas = dep.Status.ReadyReplicas
	if dep.Status.ReadyReplicas >= replicas {
		obj.Status.Phase = "Ready"
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonChildReady, "smb deployment ready")
	} else {
		obj.Status.Phase = "Progressing"
		obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonChildNotReady, "awaiting replicas")
	}
	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: defaultRequeue}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *SmbServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "SmbServer"
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("smbserver-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.SmbServer{}).
		Owns(&appsv1.Deployment{}).
		Named("SmbServer").
		Complete(r)
}
