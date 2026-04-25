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
// cryptographically random token, stores the hash in a child Secret
// and returns the plaintext exactly once via Status.RawTokenSecret.
// The next reconcile scrubs RawTokenSecret.
//
// The reconciler also enforces rotation: if Spec.RotationPeriod is set
// and Status.LastRotatedAt is older than the period, a new token is
// minted and re-delivered via Status.RawTokenSecret.
type ApiTokenReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the token Secret and manages rotation.
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
		obj.Status.Phase = "Revoked"
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
	now := metav1.NewTime(time.Now().UTC())

	// Scrub the one-shot RawTokenSecret on every reconcile that comes
	// in *after* a delivery. We detect delivery by a non-empty value
	// in status on a spec that has already been successfully reconciled.
	if obj.Status.RawTokenSecret != "" && obj.Status.Phase == "Active" {
		obj.Status.RawTokenSecret = ""
		if err := r.Client.Status().Update(ctx, &obj); err != nil && !apierrors.IsConflict(err) {
			result = "error"
			return ctrl.Result{}, err
		}
		logger.V(1).Info("apitoken: scrubbed one-shot raw token from status")
	}

	// Decide whether we need to mint (first time) or rotate.
	mustMint := obj.Status.TokenID == ""
	if !mustMint && obj.Spec.RotationPeriod != "" {
		if d, err := time.ParseDuration(obj.Spec.RotationPeriod); err == nil && d > 0 {
			if obj.Status.LastRotatedAt == nil || time.Since(obj.Status.LastRotatedAt.Time) > d {
				mustMint = true
				logger.V(1).Info("apitoken: rotation due", "period", d.String())
			}
		}
	}

	// Check expiry.
	if obj.Spec.ExpiresAt != nil && time.Now().After(obj.Spec.ExpiresAt.Time) {
		obj.Status.Phase = "Expired"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "Expired", "token past expiry")
		if err := r.Client.Status().Update(ctx, &obj); err != nil && !apierrors.IsConflict(err) {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: defaultRequeue}, nil
	}

	var sec corev1.Secret
	getErr := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: secretName}, &sec)
	if mustMint || apierrors.IsNotFound(getErr) {
		tokBytes := make([]byte, 32)
		if _, err := rand.Read(tokBytes); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		tok := hex.EncodeToString(tokBytes)
		hash := sha256.Sum256([]byte(tok))
		hashHex := hex.EncodeToString(hash[:])

		desired := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   ns,
				Name:        secretName,
				Annotations: map[string]string{"novanas.io/token-sha256": hashHex},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"tokenHash": []byte(hashHex),
			},
		}
		if err := controllerutil.SetControllerReference(&obj, &desired, r.Scheme); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		if apierrors.IsNotFound(getErr) {
			if err := r.Client.Create(ctx, &desired); err != nil && !apierrors.IsAlreadyExists(err) {
				result = "error"
				return ctrl.Result{}, err
			}
			reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonCreated, "api token minted in secret "+secretName)
		} else {
			sec.Data = desired.Data
			sec.Annotations = desired.Annotations
			if err := r.Client.Update(ctx, &sec); err != nil {
				result = "error"
				return ctrl.Result{}, err
			}
			reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonCreated, "api token rotated")
		}

		// Populate status with typed fields and deliver the raw token.
		obj.Status.TokenID = hashHex
		obj.Status.SecretRef = secretName
		obj.Status.RawTokenSecret = tok
		obj.Status.LastRotatedAt = &now
		if obj.Status.CreatedAt == nil {
			obj.Status.CreatedAt = &now
		}
	} else if getErr != nil {
		result = "error"
		return ctrl.Result{}, getErr
	}

	obj.Status.Phase = "Active"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "token available in secret "+secretName)
	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}

	// If we just delivered a raw token, requeue promptly so the next
	// reconcile can scrub it.
	requeue := defaultRequeue
	if obj.Status.RawTokenSecret != "" {
		requeue = 5 * time.Second
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *ApiTokenReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "ApiToken"
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "apitoken-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.ApiToken{}).
		Owns(&corev1.Secret{}).
		Named("ApiToken").
		Complete(r)
}
