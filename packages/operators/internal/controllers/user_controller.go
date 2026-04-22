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

// UserReconciler reconciles a User object by projecting the User into
// Keycloak via the injected KeycloakClient. Unwired reconcilers fall
// back to NoopKeycloakClient.
type UserReconciler struct {
	reconciler.BaseReconciler
	Keycloak reconciler.KeycloakClient
	Realm    string
	Recorder record.EventRecorder
}

// Reconcile ensures the user exists in Keycloak.
func (r *UserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "User", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.User
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
		logger.Info("User deleting")
		if err := kc.DeleteUser(ctx, realm, obj.Name); err != nil {
			logger.Error(err, "keycloak delete user failed")
		}
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "user removed from keycloak")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerUser); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerUser); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	if _, err := kc.EnsureUser(ctx, reconciler.KeycloakUser{Realm: realm, Username: obj.Name, Enabled: true}); err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconcileFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: defaultRequeue}, err
	}

	obj.Status.Phase = "Ready"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "user synced to keycloak")
	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonExternalSync, "user ensured in keycloak")
	return ctrl.Result{RequeueAfter: defaultRequeue}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "User"
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("user-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.User{}).
		Named("User").
		Complete(r)
}
