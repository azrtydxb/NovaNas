package controllers

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// CloudBackupTargetReconciler reconciles a CloudBackupTarget object. A
// target is a cloud-storage endpoint descriptor (S3/Azure/GCS). Reconcile
// manages lifecycle (finalizer, Ready condition). Connectivity probing is
// future work wired via StorageClient.
type CloudBackupTargetReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures finalizer + Ready condition for CloudBackupTarget.
func (r *CloudBackupTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "CloudBackupTarget", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.CloudBackupTarget
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("CloudBackupTarget deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "cloud backup target removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerCloudBackupTarget); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerCloudBackupTarget); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}
	obj.Status.Phase = "Ready"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "target ready")
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
func (r *CloudBackupTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "CloudBackupTarget"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "cloudbackuptarget-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.CloudBackupTarget{}).
		Named("CloudBackupTarget").
		Complete(r)
}
