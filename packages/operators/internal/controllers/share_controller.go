package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// ShareReconciler reconciles a Share object. A Share is a multi-protocol
// export binding; the reconciler renders a ConfigMap containing both the
// NFS /etc/exports line and the Samba-share stanza. The SMB and NFS server
// reconcilers reload their runtime from these ConfigMaps.
type ShareReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the export ConfigMap for the Share.
func (r *ShareReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "Share", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.Share
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("Share deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "share removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerShare); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerShare); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Render export snippets.
	var exports, smbConf strings.Builder
	if obj.Spec.Protocols.Nfs != nil {
		nets := strings.Join(obj.Spec.Protocols.Nfs.AllowedNetworks, ",")
		if nets == "" {
			nets = "*"
		}
		fmt.Fprintf(&exports, "%s %s(rw,sync,%s)\n", obj.Spec.Path, nets, obj.Spec.Protocols.Nfs.Squash)
	}
	if obj.Spec.Protocols.Smb != nil {
		fmt.Fprintf(&smbConf, "[%s]\n path = %s\n read only = no\n shadow: copies = %v\n",
			obj.Name, obj.Spec.Path, obj.Spec.Protocols.Smb.ShadowCopies)
	}

	ns := obj.Namespace
	if ns == "" {
		ns = "novanas-system"
	}
	cmName := "share-" + obj.Name
	cm := corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: cmName}}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, &cm, func() error {
		cm.Data = map[string]string{
			"exports":   exports.String(),
			"smb.conf":  smbConf.String(),
			"dataset":   obj.Spec.Dataset,
			"path":      obj.Spec.Path,
		}
		return controllerutil.SetControllerReference(&obj, &cm, r.Scheme)
	})
	if err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconcileFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonChildEnsured, "export configmap "+string(op))
	}

	// Prevent unused-import error when Get fails out.
	_ = types.NamespacedName{}

	obj.Status.Phase = "Ready"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "share exported")
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
func (r *ShareReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "Share"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "share-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.Share{}).
		Owns(&corev1.ConfigMap{}).
		Named("Share").
		Complete(r)
}
