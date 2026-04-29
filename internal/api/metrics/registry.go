// Package metrics exposes Prometheus collectors for nova-api.
//
// The package intentionally keeps three concerns separate:
//
//  1. A Registry construction helper that bundles the standard Go-runtime
//     and process collectors with the nova-specific collectors.
//  2. An HTTP middleware (Middleware) that records request counts and
//     latency on the chi router — using the matched route pattern as the
//     "path" label so cardinality stays bounded.
//  3. Two domain collectors: the JobMetrics group (pushed-from-worker)
//     and the ZFSCollector (pulled periodically from a goroutine).
//
// The registry is private to nova-api: handlers and the worker reach
// the metrics through the small constructor functions exported here, and
// /metrics is served via promhttp.HandlerFor on this Registry.
package metrics
