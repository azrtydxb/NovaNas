// Package reconciler hosts shared building blocks embedded by every NovaNas
// controller. Keeping this here means we can change logging / metrics / client
// wiring in one place instead of touching 50+ controller files.
package reconciler

import (
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/azrtydxb/novanas/packages/operators/internal/metrics"
)

// BaseReconciler bundles the common dependencies required by every NovaNas
// controller. Concrete reconcilers embed this struct.
type BaseReconciler struct {
	// Client is the split-cache K8s client provided by controller-runtime.
	Client client.Client
	// Scheme is the runtime scheme registered with the manager.
	Scheme *runtime.Scheme
	// Log is the base logger; controllers derive per-request sub-loggers.
	Log logr.Logger
	// ControllerName is used as the value of the "controller" label on metrics.
	ControllerName string
}

// ObserveReconcile records a reconcile attempt's outcome and duration. Call
// it via defer at the top of Reconcile:
//
//	defer r.ObserveReconcile(time.Now(), &result, &err)
func (r *BaseReconciler) ObserveReconcile(start time.Time, resultLabel string) {
	metrics.ReconcileDurationSeconds.WithLabelValues(r.ControllerName).Observe(time.Since(start).Seconds())
	metrics.ReconcileTotal.WithLabelValues(r.ControllerName, resultLabel).Inc()
}
