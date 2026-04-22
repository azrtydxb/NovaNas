package controllers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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

// ApiTokenReconciler reconciles an ApiToken. On create it generates a
// cryptographically random token, stores the cleartext in a child Secret
// (so the operator can hand it out to the user once) and keeps the SHA-256
// hash in the Secret annotations for subsequent verification. The token
// itself never lands in status.
type ApiTokenReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the token Secret.
func (r *ApiTokenReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "ApiToken", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.ApiToken
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("ApiToken deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "api token revoked")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerApiToken); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerApiToken); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	ns := obj.Namespace
	if ns == "" {
		ns = "novanas-system"
	}
	secretName := "apitoken-" + obj.Name
	var sec corev1.Secret
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: secretName}, &sec)
	if apierrors.IsNotFound(err) {
		tokBytes := make([]byte, 32)
		_, _ = rand.Read(tokBytes)
		tok := hex.EncodeToString(tokBytes)
		hash := sha256.Sum256([]byte(tok))
		sec = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   ns,
				Name:        secretName,
				Annotations: map[string]string{"novanas.io/token-sha256": hex.EncodeToString(hash[:])},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{"token": []byte(tok)},
		}
		if err := controllerutil.SetControllerReference(&obj, &sec, r.Scheme); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		if err := r.Client.Create(ctx, &sec); err != nil && !apierrors.IsAlreadyExists(err) {
			result = "error"
			return ctrl.Result{}, err
		}
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonCreated, "api token minted in secret "+secretName)
	} else if err != nil {
		result = "error"
		return ctrl.Result{}, err
	}

	obj.Status.Phase = "Ready"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "token available in secret "+secretName)
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
func (r *ApiTokenReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "ApiToken"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "apitoken-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.ApiToken{}).
		Owns(&corev1.Secret{}).
		Named("ApiToken").
		Complete(r)
}
