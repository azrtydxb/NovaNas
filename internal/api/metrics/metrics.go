package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics bundles the registry, the HTTP request middleware, and the
// per-domain collectors for nova-api.
type Metrics struct {
	Registry *prometheus.Registry
	HTTP     *HTTPMetrics
	Jobs     *JobMetrics
}

// New builds a fresh registry pre-populated with the Go-runtime and
// process collectors plus the HTTP and job metric groups.
//
// The ZFS collector is registered separately via RegisterZFS so callers
// that don't have a pool/dataset manager available (e.g. tests) can omit
// it cleanly.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	httpM := newHTTPMetrics()
	httpM.MustRegister(reg)

	jobsM := newJobMetrics()
	jobsM.MustRegister(reg)

	return &Metrics{
		Registry: reg,
		HTTP:     httpM,
		Jobs:     jobsM,
	}
}

// Handler returns the http.Handler that serves /metrics in Prometheus
// text exposition format. ErrorHandling is set to ContinueOnError so a
// flaky individual collector doesn't blow up the entire scrape.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.ContinueOnError,
	})
}
