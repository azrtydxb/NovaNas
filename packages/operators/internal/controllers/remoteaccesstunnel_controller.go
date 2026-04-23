package controllers

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

const finalizerRemoteAccessTunnel = reconciler.FinalizerPrefix + "remoteaccesstunnel"

// RemoteAccessTunnelReconciler projects the tunnel into a novaedge Tunnel
// CR. Falls back to a ConfigMap snapshot when the novaedge CRD is absent
// so the host agent can still install the systemd unit.
type RemoteAccessTunnelReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the projected tunnel resource exists.
func (r *RemoteAccessTunnelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "RemoteAccessTunnel", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.RemoteAccessTunnel
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		// Ask the host to stop the systemd unit for this tunnel before
		// removing the finalizer; best-effort (no hostagent client here,
		// the applier DaemonSet watches the ConfigMap for state=absent).
		_, _ = ensureConfigMap(ctx, r.Client, "novanas-system", childName(obj.Name, "tunnel-snapshot"), &obj, map[string]string{
			"tunnel.yaml": "state: absent\nname: " + obj.Name + "\n",
			"hash":        hashBytes([]byte("absent:" + obj.Name)),
		}, map[string]string{"novanas.io/kind": "RemoteAccessTunnel"})
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerRemoteAccessTunnel); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerRemoteAccessTunnel); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "projecting tunnel")
	obj.Status.Phase = "Reconciling"

	port := int32(51820)
	if obj.Spec.Endpoint.Port > 0 {
		port = obj.Spec.Endpoint.Port
	}
	rendered := fmt.Sprintf("type: %s\nendpoint: %s:%d\n", obj.Spec.Type, obj.Spec.Endpoint.Hostname, port)
	h := hashBytes([]byte(rendered))

	_, _ = ensureConfigMap(ctx, r.Client, "novanas-system", childName(obj.Name, "tunnel-snapshot"), &obj, map[string]string{
		"tunnel.yaml": rendered,
		"hash":        h,
	}, map[string]string{"novanas.io/kind": "RemoteAccessTunnel"})

	gvk := schema.GroupVersionKind{Group: "novaedge.io", Version: "v1alpha1", Kind: "Tunnel"}
	err := ensureUnstructured(ctx, r.Client, gvk, "novanas-system", childName(obj.Name, "tunnel"), func(u *unstructuredType) {
		setSpec(u, map[string]interface{}{
			"owner":    obj.Name,
			"type":     string(obj.Spec.Type),
			"endpoint": obj.Spec.Endpoint.Hostname,
			"port":     int64(port),
			"hash":     h,
		})
	})
	switch err {
	case nil:
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "tunnel projected")
		obj.Status.Phase = "Connected"
		now := metav1.Now()
		obj.Status.ConnectedAt = &now
	case errKindMissing:
		logger.V(1).Info("novaedge Tunnel CRD absent -- ConfigMap-only reconcile")
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonAwaitingExternal, "novaedge CRD absent; tunnel staged in ConfigMap")
		obj.Status.Phase = "Pending"
	default:
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "ProjectionFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}
	obj.Status.AppliedConfigHash = h
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "RemoteAccessTunnel reconciled")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *RemoteAccessTunnelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "RemoteAccessTunnel"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.RemoteAccessTunnel{}).
		Named("RemoteAccessTunnel").
		Complete(r)
}
