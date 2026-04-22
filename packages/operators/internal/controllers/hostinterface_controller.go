package controllers

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

const finalizerHostInterface = reconciler.FinalizerPrefix + "hostinterface"

// HostInterfaceReconciler projects a HostInterface into an nmstate
// ConfigMap and forwards to NetworkClient (no-op default).
type HostInterfaceReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
	Network  reconciler.NetworkClient
}

// Reconcile ensures the host-interface projection.
func (r *HostInterfaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "HostInterface", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.HostInterface
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerHostInterface); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerHostInterface); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "projecting host interface")
	obj.Status.Phase = "Reconciling"

	net := r.Network
	if net == nil {
		net = reconciler.NoopNetworkClient{}
	}
	state := fmt.Sprintf("interfaces:\n  - name: %s\n    type: ethernet\n    state: up\n", obj.Name)
	if _, err := ensureConfigMap(ctx, r.Client, "novanas-system", childName(obj.Name, "hif-nmstate"), &obj, map[string]string{"state.yaml": state}, map[string]string{"novanas.io/kind": "HostInterface"}); err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "ConfigMapFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}
	rev, err := net.ApplyState(ctx, obj.Name, []byte(state))
	if err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "NmstateFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	logger.V(1).Info("host interface applied", "revision", rev)
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "host interface projected (rev "+rev+")")
	obj.Status.Phase = "Ready"
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "HostInterface ready")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *HostInterfaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "HostInterface"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.HostInterface{}).
		Named("HostInterface").
		Complete(r)
}
