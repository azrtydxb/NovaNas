// Package metrics registers the NovaNas operator's Prometheus metrics with the
// controller-runtime registry so they are served on the manager's metrics port.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ReconcileTotal counts reconcile invocations per controller and outcome.
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "novanas_reconcile_total",
			Help: "Total reconcile attempts per controller and result.",
		},
		[]string{"controller", "result"},
	)

	// ReconcileDurationSeconds measures reconcile wall-time.
	ReconcileDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "novanas_reconcile_duration_seconds",
			Help:    "Reconcile wall-time in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"controller"},
	)
)

func init() {
	ctrlmetrics.Registry.MustRegister(ReconcileTotal, ReconcileDurationSeconds)
}
