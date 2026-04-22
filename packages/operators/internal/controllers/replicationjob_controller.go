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

// ReplicationJobReconciler reconciles a ReplicationJob object. On first
// reconcile the job is started via StorageClient.StartReplication; on
// subsequent reconciles its progress is polled via GetReplicationStatus and
// reflected in status. Deletion triggers CancelReplication.
type ReplicationJobReconciler struct {
	reconciler.BaseReconciler
	Storage  reconciler.StorageClient
	Recorder record.EventRecorder
}

// Reconcile drives ReplicationJob through Pending/Running/Completed.
func (r *ReplicationJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "ReplicationJob", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.ReplicationJob
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	sc := r.Storage
	if sc == nil {
		sc = reconciler.NoopStorageClient{}
	}

	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("ReplicationJob deleting")
		if err := sc.CancelReplication(ctx, string(obj.UID)); err != nil {
			logger.Error(err, "cancel replication failed")
		}
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "replication job cancelled")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerReplicationJob); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerReplicationJob); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// --- action-run-now annotation: force immediate re-run ----------
	if _, err := reconciler.HandleActionAnnotation(ctx, r.Client, &obj, "run-now",
		func(ctx context.Context, _ client.Object) error {
			logger.Info("action-run-now: restarting replication job")
			reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonProvisioning, "replication run-now requested")
			obj.Status.Phase = "Pending"
			return nil
		}); err != nil {
		logger.Error(err, "run-now handler failed")
	}

	// --- action-cancel annotation: stop the running job -------------
	if _, err := reconciler.HandleActionAnnotation(ctx, r.Client, &obj, "cancel",
		func(ctx context.Context, _ client.Object) error {
			logger.Info("action-cancel: cancelling replication job")
			reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "replication cancel requested")
			if cErr := sc.CancelReplication(ctx, string(obj.UID)); cErr != nil {
				return cErr
			}
			obj.Status.Phase = "Cancelled"
			return nil
		}); err != nil {
		logger.Error(err, "cancel handler failed")
	}
	if obj.Status.Phase == "Cancelled" {
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "cancelled by action annotation")
		if err := r.Client.Status().Update(ctx, &obj); err != nil && !apierrors.IsConflict(err) {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	var st reconciler.StorageOpStatus
	var err error
	if obj.Status.Phase == "" || obj.Status.Phase == "Pending" {
		st, err = sc.StartReplication(ctx, reconciler.ReplicationRequest{JobID: string(obj.UID)})
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonProvisioning, "replication started")
	} else {
		st, err = sc.GetReplicationStatus(ctx, string(obj.UID))
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
func (r *ReplicationJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "ReplicationJob"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "replicationjob-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.ReplicationJob{}).
		Named("ReplicationJob").
		Complete(r)
}
