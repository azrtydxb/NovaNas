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

const finalizerClusterNetwork = reconciler.FinalizerPrefix + "clusternetwork"

// ClusterNetworkReconciler is a cluster-singleton that snapshots the
// desired cluster-wide network layout into a ConfigMap for consumer
// components (CNI + novaedge) to read.
type ClusterNetworkReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the cluster-network snapshot ConfigMap exists.
func (r *ClusterNetworkReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "ClusterNetwork", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.ClusterNetwork
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerClusterNetwork); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerClusterNetwork); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "rendering cluster network snapshot")
	obj.Status.Phase = "Reconciling"

	// Resolve an effective MTU. "auto" and unset both map to 1450 — the
	// standard Geneve-compatible value assumed by novaedge when no
	// explicit size is configured.
	var effectiveMTU int32 = 1450
	if obj.Spec.Mtu != nil {
		if obj.Spec.Mtu.Type == 0 /*intstr.Int*/ && obj.Spec.Mtu.IntValue() > 0 {
			effectiveMTU = int32(obj.Spec.Mtu.IntValue())
		}
	}

	overlay := "none"
	egress := ""
	if obj.Spec.Overlay != nil {
		overlay = string(obj.Spec.Overlay.Type)
		egress = obj.Spec.Overlay.EgressInterface
	}
	defaultDeny := false
	if obj.Spec.Policy != nil {
		defaultDeny = obj.Spec.Policy.DefaultDeny
	}
	rendered := fmt.Sprintf("name: %s\npodCidr: %s\nserviceCidr: %s\noverlay: %s\negress: %s\nmtu: %d\ndefaultDeny: %t\n",
		obj.Name, obj.Spec.PodCidr, obj.Spec.ServiceCidr, overlay, egress, effectiveMTU, defaultDeny)
	data := map[string]string{
		"cluster-network.yaml": rendered,
		"hash":                 hashBytes([]byte(rendered)),
	}
	if _, err := ensureConfigMap(ctx, r.Client, "novanas-system", childName(obj.Name, "clusternet"), &obj, data, map[string]string{"novanas.io/kind": "ClusterNetwork"}); err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "ConfigMapFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}
	logger.V(1).Info("cluster network snapshot written", "effectiveMTU", effectiveMTU)
	obj.Status.EffectiveMtu = effectiveMTU
	obj.Status.AppliedConfigHash = hashBytes([]byte(rendered))
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "cluster network snapshot current")
	obj.Status.Phase = "Active"
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "ClusterNetwork ready")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *ClusterNetworkReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "ClusterNetwork"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.ClusterNetwork{}).
		Named("ClusterNetwork").
		Complete(r)
}
