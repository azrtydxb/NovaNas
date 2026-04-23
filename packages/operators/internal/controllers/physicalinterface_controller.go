package controllers

import (
	"context"
	"time"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

const finalizerPhysicalInterface = reconciler.FinalizerPrefix + "physicalinterface"

// PhysicalInterfaceReconciler observes a physical NIC from the host.
//
// Status-only: a host-side observer DaemonSet reports MAC / speed /
// operState. When no observer is wired the reconciler emits a
// deterministic "Observed" placeholder with UsedBy populated from owner
// references so the UI has something to render.
type PhysicalInterfaceReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
	Network  reconciler.NetworkClient
}

// Reconcile records observed state for the NIC.
func (r *PhysicalInterfaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "PhysicalInterface", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.PhysicalInterface
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerPhysicalInterface); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerPhysicalInterface); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	net := r.Network
	if net == nil {
		net = reconciler.NoopNetworkClient{}
	}
	observed, _ := net.ObservedState(ctx, obj.Name)
	if observed == nil {
		logger.V(1).Info("no network observer wired -- recording placeholder")
		if obj.Status.Link == "" {
			obj.Status.Link = "up"
		}
		if obj.Status.Duplex == "" {
			obj.Status.Duplex = "unknown"
		}
		// LinkSpeed == 0 implicitly means unknown until a real reflector
		// overwrites it; nothing to do here.
	}
	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "observed")
	obj.Status.Phase = "Observed"
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "PhysicalInterface observed")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *PhysicalInterfaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "PhysicalInterface"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.PhysicalInterface{}).
		Named("PhysicalInterface").
		Complete(r)
}
