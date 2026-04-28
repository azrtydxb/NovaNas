package middleware

import (
	"log/slog"
	"net/http"
)

func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic in handler",
						"panic", rec,
						"path", r.URL.Path,
						"requestID", RequestIDOf(r.Context()),
					)
					WriteError(w, http.StatusInternalServerError, "internal_error", "")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
