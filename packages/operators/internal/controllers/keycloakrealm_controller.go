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

// KeycloakRealmReconciler reconciles a KeycloakRealm object by applying a
// realm representation to Keycloak via the injected KeycloakClient.
type KeycloakRealmReconciler struct {
	reconciler.BaseReconciler
	Keycloak reconciler.KeycloakClient
	Recorder record.EventRecorder
}

// Reconcile ensures the realm exists in Keycloak.
func (r *KeycloakRealmReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "KeycloakRealm", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.KeycloakRealm
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	kc := r.Keycloak
	if kc == nil {
		kc = reconciler.NoopKeycloakClient{}
	}

	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("KeycloakRealm deleting")
		if err := kc.DeleteRealm(ctx, obj.Name); err != nil {
			logger.Error(err, "keycloak delete realm failed")
		}
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "realm removed from keycloak")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerKeycloakRealm); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerKeycloakRealm); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	if err := kc.EnsureRealm(ctx, reconciler.KeycloakRealmConfig{Name: obj.Name, Enabled: true}); err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconcileFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: defaultRequeue}, err
	}

	obj.Status.Phase = "Ready"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "realm synced")
	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonExternalSync, "realm ensured in keycloak")
	return ctrl.Result{RequeueAfter: defaultRequeue}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *KeycloakRealmReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "KeycloakRealm"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "keycloakrealm-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.KeycloakRealm{}).
		Named("KeycloakRealm").
		Complete(r)
}
