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

const finalizerBond = reconciler.FinalizerPrefix + "bond"

// BondReconciler reconciles a Bond object.
//
// Reconcile projects the Bond spec into a rendered nmstate YAML document,
// stores a copy in a ConfigMap for auditability, and hands the bytes off to
// the configured NetworkClient (ConfigMap-backed in production, noop in
// tests). On deletion the finalizer teardown asks the host to shut down
// the bond member by publishing a state=absent document.
type BondReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
	Network  reconciler.NetworkClient
}

// Reconcile projects Bond into an nmstate ConfigMap and tracks status.
func (r *BondReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "Bond", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.Bond
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	net := r.Network
	if net == nil {
		net = reconciler.NoopNetworkClient{}
	}

	if !obj.DeletionTimestamp.IsZero() {
		// Tear the bond down before letting Kubernetes delete the CR.
		teardown := "interfaces:\n  - name: " + obj.Name + "\n    type: bond\n    state: absent\n"
		if _, err := net.ApplyState(ctx, obj.Name, []byte(teardown)); err != nil {
			logger.Error(err, "bond teardown failed; proceeding with finalizer removal to avoid stuck delete")
		}
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerBond); err != nil {
			return ctrl.Result{}, err
		}
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "Bond removed")
		return ctrl.Result{}, nil
	}

	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerBond); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "projecting bond state")
	obj.Status.Phase = "Reconciling"

	state := renderBondNmstate(&obj)
	stateBytes := []byte(state)
	cmName := childName(obj.Name, "nmstate")
	if _, err := ensureConfigMap(ctx, r.Client, "novanas-system", cmName, &obj, map[string]string{"state.yaml": state, "hash": hashBytes(stateBytes)}, map[string]string{"novanas.io/kind": "Bond"}); err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "ConfigMapFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		reconciler.EmitWarning(r.Recorder, &obj, reconciler.EventReasonFailed, err.Error())
		return ctrl.Result{}, err
	}

	rev, err := net.ApplyState(ctx, obj.Name, stateBytes)
	if err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "NmstateFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	logger.V(1).Info("nmstate applied", "revision", rev)

	obj.Status.AppliedConfigHash = hashBytes(stateBytes)
	// Active members default to the full member list until observedState
	// reporting is wired. A real host reflector will override this via a
	// watch.
	obj.Status.ActiveMembers = append([]string(nil), obj.Spec.Interfaces...)
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "bond projected (rev "+rev+")")
	obj.Status.Phase = "Active"
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "Bond ready")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *BondReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "Bond"
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.Bond{}).
		Named("Bond").
		Complete(r)
}
