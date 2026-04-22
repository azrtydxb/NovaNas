package controllers

import (
	"context"
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

// CertificateReconciler reconciles a Certificate object. On first
// reconcile it calls CertificateIssuer.Issue and stores the resulting
// material in a child Secret named <cert-name>-tls. The Secret carries
// an owner reference so it is garbage-collected when the Certificate
// is deleted; the issuer Revoke hook is invoked via finalizer.
type CertificateReconciler struct {
	reconciler.BaseReconciler
	Issuer   reconciler.CertificateIssuer
	Recorder record.EventRecorder
}

// Reconcile issues (or revokes) the Certificate.
func (r *CertificateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "Certificate", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.Certificate
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	iss := r.Issuer
	if iss == nil {
		iss = reconciler.NoopCertificateIssuer{}
	}

	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("Certificate deleting")
		if err := iss.Revoke(ctx, obj.Name); err != nil {
			logger.Error(err, "revoke failed; continuing")
		}
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "certificate revoked")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerCertificate); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerCertificate); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Ensure the Secret holding the issued material. Skip issuance if it
	// already exists and appears valid.
	secretName := obj.Name + "-tls"
	var sec corev1.Secret
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: obj.Namespace, Name: secretName}, &sec)
	if apierrors.IsNotFound(err) {
		bundle, ierr := iss.Issue(ctx, reconciler.CertificateRequest{Name: obj.Name, CommonName: obj.Name})
		if ierr != nil {
			obj.Status.Phase = "Failed"
			obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconcileFailed, ierr.Error())
			_ = r.Client.Status().Update(ctx, &obj)
			result = "error"
			return ctrl.Result{RequeueAfter: defaultRequeue}, ierr
		}
		sec = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: obj.Namespace, Name: secretName},
			Type:       corev1.SecretTypeTLS,
			Data: map[string][]byte{
				corev1.TLSCertKey:       bundle.CertPEM,
				corev1.TLSPrivateKeyKey: bundle.KeyPEM,
				"ca.crt":                bundle.CAPEM,
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
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonCreated, "certificate issued")
	} else if err != nil {
		result = "error"
		return ctrl.Result{}, err
	}

	obj.Status.Phase = "Ready"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "certificate issued")
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
func (r *CertificateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "Certificate"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "certificate-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.Certificate{}).
		Owns(&corev1.Secret{}).
		Named("Certificate").
		Complete(r)
}
