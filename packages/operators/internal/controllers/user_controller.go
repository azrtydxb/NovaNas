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
//
// In addition to Keycloak projection, the reconciler provisions a
// per-tenant OpenBao policy and kubernetes-auth role at EnsureUser time
// (and revokes them on DeleteUser). When OpenBao is nil, a no-op client
// is used so the controller remains runnable in dev/test environments
// without an OpenBao instance wired up.
type UserReconciler struct {
	reconciler.BaseReconciler
	Keycloak reconciler.KeycloakClient
	Realm    string
	Recorder record.EventRecorder

	// OpenBao (optional) provisions a per-tenant policy + kubernetes-auth
	// role. When nil, NoopOpenBaoClient is used.
	OpenBao reconciler.OpenBaoClient
	// OpenBaoPolicyTemplate optionally overrides the default per-tenant
	// policy HCL template; when empty DefaultTenantPolicyTemplate is used.
	OpenBaoPolicyTemplate string
	// OpenBaoBoundNamespace is the namespace the tenant service account
	// lives in (defaults to the User object's namespace at reconcile time).
	OpenBaoBoundNamespace string
}

// Reconcile ensures the user exists in Keycloak and has a per-tenant
// OpenBao policy + kubernetes-auth role.
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
	ob := r.OpenBao
	if ob == nil {
		ob = reconciler.NoopOpenBaoClient{}
	}
	realm := r.Realm
	if realm == "" {
		realm = "novanas"
	}

	policyName := reconciler.TenantPolicyName(obj.Name)
	roleName := reconciler.TenantAuthRoleName(obj.Name)

	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("User deleting")
		delUser := obj.Spec.Username
		if delUser == "" {
			delUser = obj.Name
		}
		delRealm := realm
		if obj.Spec.Realm != "" {
			delRealm = obj.Spec.Realm
		}
		if err := kc.DeleteUser(ctx, delRealm, delUser); err != nil {
			logger.Error(err, "keycloak delete user failed")
		}
		if err := ob.DeleteAuthRole(ctx, roleName); err != nil {
			logger.Error(err, "openbao delete auth role failed")
		}
		if err := ob.DeletePolicy(ctx, policyName); err != nil {
			logger.Error(err, "openbao delete policy failed")
		}
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "user removed from keycloak and openbao")
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

	// Prefer the spec username when provided; fall back to the CR name.
	username := obj.Spec.Username
	if username == "" {
		username = obj.Name
	}
	enabled := true
	if obj.Spec.Enabled != nil {
		enabled = *obj.Spec.Enabled
	}
	effRealm := realm
	if obj.Spec.Realm != "" {
		effRealm = obj.Spec.Realm
	}
	keycloakID, err := kc.EnsureUser(ctx, reconciler.KeycloakUser{
		Realm:    effRealm,
		Username: username,
		Email:    obj.Spec.Email,
		Groups:   obj.Spec.Groups,
		Enabled:  enabled,
	})
	if err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconcileFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: defaultRequeue}, err
	}
	obj.Status.KeycloakID = keycloakID

	// Per-tenant OpenBao policy + auth role.
	hcl, err := reconciler.RenderTenantPolicy(r.OpenBaoPolicyTemplate, obj.Name)
	if err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconcileFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: defaultRequeue}, err
	}
	if err := ob.EnsurePolicy(ctx, reconciler.OpenBaoPolicy{Name: policyName, HCL: hcl}); err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconcileFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: defaultRequeue}, err
	}
	boundNS := r.OpenBaoBoundNamespace
	if boundNS == "" {
		boundNS = obj.Namespace
	}
	if err := ob.EnsureAuthRole(ctx, reconciler.OpenBaoAuthRole{
		Name:                roleName,
		BoundServiceAccount: obj.Name,
		BoundNamespace:      boundNS,
		Policies:            []string{policyName},
		TTLSeconds:          3600,
		MaxTTLSeconds:       86400,
	}); err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconcileFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: defaultRequeue}, err
	}

	obj.Status.Phase = "Active"
	if !enabled {
		obj.Status.Phase = "Disabled"
	}
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "user synced to keycloak and openbao")
	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonExternalSync, "user ensured in keycloak and openbao")
	return ctrl.Result{RequeueAfter: defaultRequeue}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "User"
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "user-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.User{}).
		Named("User").
		Complete(r)
}
