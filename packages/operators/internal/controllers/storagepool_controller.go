package controllers

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// StoragePoolReconciler reconciles a StoragePool object.
//
// StoragePool is a cluster-scoped, abstract descriptor of a bag-of-disks. The
// controller is therefore a status-tracker: it does not provision anything of
// its own, but instead aggregates the observed state of member Disks (driven
// by DiskReconciler) and reflects pool-level capacity / health in status.
type StoragePoolReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile updates StoragePool status from observed member Disks.
func (r *StoragePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "StoragePool", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var sp novanasv1alpha1.StoragePool
	if err := r.Client.Get(ctx, req.NamespacedName, &sp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !sp.DeletionTimestamp.IsZero() {
		logger.Info("StoragePool deleting")
		reconciler.Emit(r.Recorder, &sp, reconciler.EventReasonDeleted, "storage pool deleted")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &sp, reconciler.FinalizerStoragePool); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &sp, reconciler.FinalizerStoragePool); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Aggregate disk membership (best-effort: list by pool label if present).
	var disks novanasv1alpha1.DiskList
	if err := r.Client.List(ctx, &disks); err != nil {
		logger.V(1).Info("disk list failed (tolerating)", "err", err.Error())
	}
	var count, capacity, used int32
	var capBytes, usedBytes int64
	for i := range disks.Items {
		d := &disks.Items[i]
		if d.Spec.Pool == sp.Name {
			count++
			capBytes += d.Status.SizeBytes
			_ = capacity
			_ = used
			_ = usedBytes
		}
	}

	sp.Status.DiskCount = count
	sp.Status.CapacityBytes = capBytes
	if sp.Status.Phase == "" {
		sp.Status.Phase = "Ready"
	}
	sp.Status.Conditions = reconciler.MarkReady(sp.Status.Conditions, sp.Generation,
		reconciler.ReasonReconciled, fmt.Sprintf("pool has %d member disks", count))

	if err := r.Client.Status().Update(ctx, &sp); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &sp, reconciler.EventReasonReady, "storage pool reconciled")
	return ctrl.Result{RequeueAfter: defaultRequeue}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *StoragePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "StoragePool"
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "storagepool-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.StoragePool{}).
		// Re-reconcile a pool whenever any of its member Disks
		// change. Without this watch the StoragePool's diskCount /
		// capacity stay at zero until something else (the 30s
		// requeue or a user-initiated update) wakes the controller.
		Watches(
			&novanasv1alpha1.Disk{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
				d, ok := obj.(*novanasv1alpha1.Disk)
				if !ok || d.Spec.Pool == "" {
					return nil
				}
				return []reconcile.Request{{
					NamespacedName: types.NamespacedName{Name: d.Spec.Pool},
				}}
			}),
		).
		Named("StoragePool").
		Complete(r)
}
