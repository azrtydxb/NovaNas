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

// BucketUserReconciler reconciles a BucketUser object. On first reconcile
// it generates S3-style access-key/secret-key credentials and stores
// them in an owned Secret. The Secret carries the SHA-256 hash of the
// secret key in its annotations so the reconciler can detect rotation
// without keeping the cleartext in status.
type BucketUserReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the credentials Secret for BucketUser.
func (r *BucketUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "BucketUser", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.BucketUser
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("BucketUser deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "bucket user removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerBucketUser); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerBucketUser); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	ns := obj.Namespace
	if ns == "" {
		ns = "novanas-system"
	}
	secretName := "bucketuser-" + obj.Name
	var sec corev1.Secret
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: secretName}, &sec)
	if apierrors.IsNotFound(err) {
		ak := randomHex(8)
		sk := randomHex(20)
		hash := sha256.Sum256([]byte(sk))
		sec = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      secretName,
				Annotations: map[string]string{
					"novanas.io/secret-key-sha256": hex.EncodeToString(hash[:]),
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"accessKey": []byte(ak),
				"secretKey": []byte(sk),
			},
		}
		if err := controllerutil.SetControllerReference(&obj, &sec, r.Scheme); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		if err := r.Client.Create(ctx, &sec); err != nil && !apierrors.IsAlreadyExists(err) {
			result = "error"
			return ctrl.Result{}, err
		}
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonCreated, "credentials minted")
	} else if err != nil {
		result = "error"
		return ctrl.Result{}, err
	}

	obj.Status.Phase = "Ready"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "credentials ready in secret "+secretName)
	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: defaultRequeue}, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SetupWithManager registers the controller with the manager.
func (r *BucketUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "BucketUser"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "bucketuser-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.BucketUser{}).
		Owns(&corev1.Secret{}).
		Named("BucketUser").
		Complete(r)
}
