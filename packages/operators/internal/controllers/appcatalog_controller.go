package controllers

import (
	"context"
	"time"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

const finalizerAppCatalog = reconciler.FinalizerPrefix + "appcatalog"

// CatalogFetcher fetches the index file of a catalog source and returns
// the list of app names advertised. The default implementation returns a
// deterministic placeholder so controllers exercise the happy path without
// external network access in tests.
type CatalogFetcher interface {
	Fetch(ctx context.Context, name string) ([]string, error)
}

// NoopCatalogFetcher returns a deterministic two-entry catalog.
type NoopCatalogFetcher struct{}

// Fetch returns a deterministic catalog listing.
func (NoopCatalogFetcher) Fetch(_ context.Context, name string) ([]string, error) {
	return []string{name + "-hello", name + "-world"}, nil
}

// AppCatalogReconciler fetches the catalog index and snapshots the listing
// into a ConfigMap that the App controller then projects into App CRs.
type AppCatalogReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
	Fetcher  CatalogFetcher
}

// Reconcile fetches the catalog index and records it.
func (r *AppCatalogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "AppCatalog", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.AppCatalog
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerAppCatalog); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerAppCatalog); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciling, "fetching catalog index")
	obj.Status.Phase = "Reconciling"

	f := r.Fetcher
	if f == nil {
		f = NoopCatalogFetcher{}
	}
	apps, err := f.Fetch(ctx, obj.Name)
	if err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "FetchFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}
	data := map[string]string{}
	for _, a := range apps {
		data["app-"+a] = a
	}
	if _, err := ensureConfigMap(ctx, r.Client, "novanas-system", childName(obj.Name, "catalog-index"), &obj, data, map[string]string{"novanas.io/kind": "AppCatalog"}); err != nil {
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "ConfigMapFailed", err.Error())
		obj.Status.Phase = "Failed"
		_ = statusUpdate(ctx, r.Client, &obj)
		return ctrl.Result{}, err
	}
	logger.V(1).Info("catalog snapshot written", "apps", len(apps))
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "catalog fetched")
	obj.Status.Phase = "Ready"
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "AppCatalog synced")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *AppCatalogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "AppCatalog"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.AppCatalog{}).
		Named("AppCatalog").
		Complete(r)
}
