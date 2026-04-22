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

// CloudBackupJobReconciler reconciles a CloudBackupJob object by delegating
// to StorageClient.StartBackup / GetBackupStatus / CancelBackup.
type CloudBackupJobReconciler struct {
	reconciler.BaseReconciler
	Storage  reconciler.StorageClient
	Recorder record.EventRecorder
}

// Reconcile drives CloudBackupJob through Pending/Running/Completed.
func (r *CloudBackupJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "CloudBackupJob", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.CloudBackupJob
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	sc := r.Storage
	if sc == nil {
		sc = reconciler.NoopStorageClient{}
	}

	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("CloudBackupJob deleting")
		if err := sc.CancelBackup(ctx, string(obj.UID)); err != nil {
			logger.Error(err, "cancel backup failed")
		}
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "cloud backup job cancelled")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerCloudBackupJob); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerCloudBackupJob); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	var st reconciler.StorageOpStatus
	var err error
	if obj.Status.Phase == "" || obj.Status.Phase == "Pending" {
		st, err = sc.StartBackup(ctx, reconciler.BackupRequest{JobID: string(obj.UID)})
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonProvisioning, "backup started")
	} else {
		st, err = sc.GetBackupStatus(ctx, string(obj.UID))
	}
	if err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconcileFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: defaultRequeue}, err
	}

	obj.Status.Phase = st.Phase
	switch st.Phase {
	case "Completed":
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, st.Message)
	case "Failed":
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconcileFailed, st.Message)
	default:
		obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, st.Message)
	}
	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	if st.Phase == "Completed" || st.Phase == "Failed" {
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *CloudBackupJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "CloudBackupJob"
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("cloudbackupjob-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.CloudBackupJob{}).
		Named("CloudBackupJob").
		Complete(r)
}
