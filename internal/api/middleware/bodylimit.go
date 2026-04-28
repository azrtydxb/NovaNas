package middleware

import "net/http"

const DefaultMaxBody = 1 << 20 // 1 MiB

// BodyLimit caps request body size at max bytes (or DefaultMaxBody when
// max <= 0). Uses http.MaxBytesReader so handlers reading the body see a
// MaxBytesError after the limit.
func BodyLimit(max int64) func(http.Handler) http.Handler {
	if max <= 0 {
		max = DefaultMaxBody
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}
