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

// GroupReconciler reconciles a Group object by projecting the Group into
// Keycloak via the injected KeycloakClient.
type GroupReconciler struct {
	reconciler.BaseReconciler
	Keycloak reconciler.KeycloakClient
	Realm    string
	Recorder record.EventRecorder
}

// Reconcile ensures the group exists in Keycloak.
func (r *GroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "Group", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.Group
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	kc := r.Keycloak
	if kc == nil {
		kc = reconciler.NoopKeycloakClient{}
	}
	realm := r.Realm
	if realm == "" {
		realm = "novanas"
	}

	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("Group deleting")
		if err := kc.DeleteGroup(ctx, realm, obj.Name); err != nil {
			logger.Error(err, "keycloak delete group failed")
		}
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "group removed from keycloak")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerGroup); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerGroup); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	if _, err := kc.EnsureGroup(ctx, reconciler.KeycloakGroup{Realm: realm, Name: obj.Name}); err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconcileFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: defaultRequeue}, err
	}

	obj.Status.Phase = "Ready"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "group synced to keycloak")
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
func (r *GroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "Group"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "group-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.Group{}).
		Named("Group").
		Complete(r)
}
