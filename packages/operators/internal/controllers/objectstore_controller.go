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

// ObjectStoreReconciler reconciles an ObjectStore object by ensuring an
// s3gw Deployment exists that fronts the NovaNas chunk engine with an S3
// API. Ready is gated on ReadyReplicas matching the desired count.
type ObjectStoreReconciler struct {
	reconciler.BaseReconciler
	Recorder       record.EventRecorder
	ContainerImage string
}

// Reconcile ensures the s3gw Deployment.
func (r *ObjectStoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "ObjectStore", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.ObjectStore
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("ObjectStore deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "object store removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerObjectStore); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerObjectStore); err != nil {
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
		img = "ghcr.io/azrtydxb/novanas/s3gw:dev"
	}
	replicas := int32(1)
	dep := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "s3gw-" + obj.Name}}
	labels := map[string]string{"app.kubernetes.io/name": "objectstore", "novanas.io/objectstore": obj.Name}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, &dep, func() error {
		dep.Spec.Replicas = &replicas
		dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		dep.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "s3gw", Image: img}}},
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
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonChildEnsured, "s3gw deployment "+string(op))
	}

	if dep.Status.ReadyReplicas >= replicas {
		obj.Status.Phase = "Ready"
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonChildReady, "object store ready")
	} else {
		obj.Status.Phase = "Progressing"
		obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonChildNotReady, "awaiting s3gw replica")
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
func (r *ObjectStoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "ObjectStore"
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "objectstore-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.ObjectStore{}).
		Owns(&appsv1.Deployment{}).
		Named("ObjectStore").
		Complete(r)
}
