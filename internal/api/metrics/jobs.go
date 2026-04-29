package metrics

import "github.com/prometheus/client_golang/prometheus"

// JobMetrics tracks the asynq-driven worker pipeline. The dispatcher
// increments Dispatched on a successful enqueue, and the worker calls
// MarkRunning / MarkFinished from its markRunning / finish helpers.
//
// JobMetrics is intentionally nil-safe: every method short-circuits when
// the receiver is nil, so the dispatcher and worker can be constructed
// without a live registry (e.g. in unit tests).
type JobMetrics struct {
	dispatched *prometheus.CounterVec
	finished   *prometheus.CounterVec
	duration   *prometheus.HistogramVec
	inFlight   *prometheus.GaugeVec
}

func newJobMetrics() *JobMetrics {
	return &JobMetrics{
		dispatched: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nova_jobs_dispatched_total",
			Help: "Jobs successfully enqueued by the dispatcher, by kind.",
		}, []string{"kind"}),
		finished: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nova_jobs_finished_total",
			Help: "Jobs reaching a terminal state in the worker, by kind and state.",
		}, []string{"kind", "state"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "nova_job_duration_seconds",
			Help: "Worker-observed job duration in seconds, by kind and state.",
			// Job durations span synchronous zfs ops (sub-second) up to
			// long-running scrubs; the upper bucket is generous on purpose.
			Buckets: []float64{0.05, 0.1, 0.5, 1, 5, 30, 60, 300, 1800},
		}, []string{"kind", "state"}),
		inFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_jobs_in_flight",
			Help: "Jobs currently executing in the worker pool, by kind.",
		}, []string{"kind"}),
	}
}

// MustRegister attaches all job metric vectors to reg.
func (j *JobMetrics) MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(j.dispatched, j.finished, j.duration, j.inFlight)
}

// Dispatched bumps the dispatcher counter for kind.
func (j *JobMetrics) Dispatched(kind string) {
	if j == nil {
		return
	}
	j.dispatched.WithLabelValues(kind).Inc()
}

// MarkRunning increments the in-flight gauge for kind. Pair with
// MarkFinished so the gauge balances out across each job's lifecycle.
func (j *JobMetrics) MarkRunning(kind string) {
	if j == nil {
		return
	}
	j.inFlight.WithLabelValues(kind).Inc()
}

// MarkFinished decrements the in-flight gauge, increments the finished
// counter, and records the job's duration histogram observation.
func (j *JobMetrics) MarkFinished(kind, state string, durationSeconds float64) {
	if j == nil {
		return
	}
	j.inFlight.WithLabelValues(kind).Dec()
	j.finished.WithLabelValues(kind, state).Inc()
	j.duration.WithLabelValues(kind, state).Observe(durationSeconds)
}
