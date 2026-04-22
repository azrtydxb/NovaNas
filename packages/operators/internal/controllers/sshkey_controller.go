package controllers

import (
	"context"
	"time"

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

// SshKeyReconciler reconciles a SshKey object by projecting the public
// key line into a cluster-wide ConfigMap consumed by the node agent when
// rendering /root/.ssh/authorized_keys.
type SshKeyReconciler struct {
	reconciler.BaseReconciler
	AuthorizedKeysConfigMap string
	Recorder                record.EventRecorder
}

// Reconcile ensures the SshKey entry in the authorized-keys ConfigMap.
func (r *SshKeyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "SshKey", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.SshKey
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("SshKey deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "ssh key removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerSshKey); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerSshKey); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	cmName := r.AuthorizedKeysConfigMap
	if cmName == "" {
		cmName = "ssh-authorized-keys"
	}
	ns := obj.Namespace
	if ns == "" {
		ns = "novanas-system"
	}
	cm := corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: cmName}}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, &cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		// Child key namespaced by the CR name so multiple SshKey CRs can
		// coexist in a single ConfigMap.
		cm.Data[obj.Name] = "# managed-by-novanas ssh key " + obj.Name + "\n"
		return nil
	})
	if err != nil {
		result = "error"
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonChildEnsured, "authorized-keys configmap "+string(op))
	}

	obj.Status.Phase = "Ready"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "ssh key projected to "+cmName)
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
func (r *SshKeyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "SshKey"
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("sshkey-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.SshKey{}).
		Named("SshKey").
		Complete(r)
}
