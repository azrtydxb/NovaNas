package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
)

// HTTPMetrics holds the request-level counters and latency histogram.
type HTTPMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func newHTTPMetrics() *HTTPMetrics {
	return &HTTPMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nova_http_requests_total",
			Help: "HTTP requests handled by nova-api, partitioned by chi-matched route pattern.",
		}, []string{"method", "path", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "nova_http_request_duration_seconds",
			Help: "HTTP request latency in seconds, partitioned by chi-matched route pattern.",
			// Bucket spread covers fast in-process replies up to slow zpool
			// command invocations (multi-second).
			Buckets: []float64{
				0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
			},
		}, []string{"method", "path"}),
	}
}

// MustRegister attaches the HTTP metric vectors to reg.
func (h *HTTPMetrics) MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(h.requests, h.duration)
}

// statusRecorder captures the response status code so the middleware
// can use it as a label after the downstream handler returns.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (sr *statusRecorder) WriteHeader(code int) {
	if !sr.wroteHeader {
		sr.status = code
		sr.wroteHeader = true
	}
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if !sr.wroteHeader {
		sr.status = http.StatusOK
		sr.wroteHeader = true
	}
	return sr.ResponseWriter.Write(b)
}

// Flush forwards http.Flusher.Flush so SSE handlers wrapped by this
// middleware can still push response bytes immediately (matches the
// existing logging middleware's behavior).
func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Middleware records request counts and durations on the chi router. The
// path label is taken from chi's matched route pattern (e.g.
// "/api/v1/pools/{name}") rather than the literal URL — that keeps label
// cardinality bounded.
//
// /metrics itself is excluded from instrumentation so each scrape doesn't
// recursively bump its own counter.
func (h *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)

		// chi populates RouteContext.RoutePattern() only after the route
		// resolves, so we read it here. For unmatched routes it returns
		// an empty string; we substitute a constant to avoid a high-
		// cardinality stream of literal URLs.
		path := chi.RouteContext(r.Context()).RoutePattern()
		if path == "" {
			path = "unmatched"
		}

		status := strconv.Itoa(sr.status)
		h.requests.WithLabelValues(r.Method, path, status).Inc()
		h.duration.WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
	})
}
