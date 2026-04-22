// Package controller provides Kubernetes controllers for NovaStor resources.
// This file manages the lifecycle of SharedFilesystem custom resources.
//
// NOTE: SharedFilesystem is deprecated. NovaNas now composes shares from
// Dataset + Share + NfsServer / SmbServer CRDs (see packages/operators/api/v1alpha1).
// The previous implementation created a `novanas-storage-filer` Deployment, but
// that image no longer exists -- the filer role has been replaced by host
// knfsd + a Samba pod wired through the `Share` and related server CRDs.
//
// This reconciler now:
//  - validates that the referenced StoragePool exists and is Ready,
//  - marks the CR Ready with a clear `Deprecated` condition and a migration
//    message pointing operators at Dataset/Share,
//  - no longer creates any Deployments or Services.
//
// New deployments should use `Dataset` (for filesystem storage) with a `Share`
// resource referencing an `NfsServer` or `SmbServer` instead.
package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novastorev1alpha1 "github.com/azrtydxb/novanas/storage/api/v1alpha1"
)

const (
	nfsPort = int32(2049)
)

// SharedFilesystemReconciler reconciles SharedFilesystem objects.
//
// Deprecated: use Dataset + Share + NfsServer/SmbServer CRDs from
// packages/operators/api/v1alpha1 instead.
type SharedFilesystemReconciler struct {
	client.Client
	// ImageRegistry / ImageTag / ImagePullPolicy / ImagePullSecrets remain on
	// the struct for source compatibility with Wave 3 call sites, but are
	// unused now that the reconciler no longer provisions a filer Deployment.
	ImageRegistry    string
	ImageTag         string
	ImagePullPolicy  string
	ImagePullSecrets []string
}

// +kubebuilder:rbac:groups=novanas.io,resources=sharedfilesystems,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=novanas.io,resources=sharedfilesystems/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=novanas.io,resources=sharedfilesystems/finalizers,verbs=update

// Reconcile handles a single reconciliation request for a SharedFilesystem.
// It validates the referenced StoragePool and emits a deprecation condition
// pointing operators at the Dataset + Share CRDs. It does NOT create any
// Deployments or Services.
func (r *SharedFilesystemReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var fs novastorev1alpha1.SharedFilesystem
	if err := r.Get(ctx, req.NamespacedName, &fs); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("reconciling SharedFilesystem (deprecated -- migrate to Dataset + Share)",
		"name", req.Name, "namespace", req.Namespace)

	// Look up the referenced StoragePool.
	var pool novastorev1alpha1.StoragePool
	poolKey := types.NamespacedName{Name: fs.Spec.Pool}
	if err := r.Get(ctx, poolKey, &pool); err != nil {
		if errors.IsNotFound(err) {
			meta.SetStatusCondition(&fs.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "PoolNotFound",
				Message:            fmt.Sprintf("StoragePool %q not found", fs.Spec.Pool),
				ObservedGeneration: fs.Generation,
			})
			fs.Status.Phase = "Pending"
			if statusErr := r.Status().Update(ctx, &fs); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	// Check pool readiness.
	if pool.Status.Phase != "Ready" {
		meta.SetStatusCondition(&fs.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "PoolNotReady",
			Message:            fmt.Sprintf("StoragePool %q is not ready (phase: %s)", fs.Spec.Pool, pool.Status.Phase),
			ObservedGeneration: fs.Generation,
		})
		fs.Status.Phase = "Pending"
		if statusErr := r.Status().Update(ctx, &fs); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Deprecation notice condition.
	meta.SetStatusCondition(&fs.Status.Conditions, metav1.Condition{
		Type:               "Deprecated",
		Status:             metav1.ConditionTrue,
		Reason:             "UseDatasetAndShare",
		Message:            "SharedFilesystem is deprecated. Use Dataset + Share + NfsServer/SmbServer from novanas.io/v1alpha1 instead.",
		ObservedGeneration: fs.Generation,
	})

	// Populate a service-style endpoint hint for clients that read it. No
	// Kubernetes resource is created -- the actual share is surfaced by the
	// Share + NfsServer/SmbServer CRD reconcilers.
	fs.Status.Endpoint = fmt.Sprintf("novanas-nfs-%s.%s.svc:%d", fs.Name, fs.Namespace, nfsPort)
	fs.Status.Phase = "Ready"

	meta.SetStatusCondition(&fs.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "FilesystemAccepted",
		Message:            "SharedFilesystem accepted (deprecated path; no resources created).",
		ObservedGeneration: fs.Generation,
	})

	if err := r.Status().Update(ctx, &fs); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager registers the SharedFilesystem controller with the manager.
func (r *SharedFilesystemReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novastorev1alpha1.SharedFilesystem{}).
		Named("sharedfilesystem").
		Complete(r)
}
