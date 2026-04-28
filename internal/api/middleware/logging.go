package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type respWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (rw *respWriter) WriteHeader(s int) {
	rw.status = s
	rw.ResponseWriter.WriteHeader(s)
}
func (rw *respWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &respWriter{ResponseWriter: w}
			next.ServeHTTP(rw, r)
			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.status,
				"bytes", rw.bytes,
				"durMS", time.Since(start).Milliseconds(),
				"requestID", RequestIDOf(r.Context()),
			)
		})
	}
}
