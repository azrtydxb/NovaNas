package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

type ctxKey struct{}

var identityCtxKey = ctxKey{}

// IdentityFromContext retrieves the identity attached by Middleware.
func IdentityFromContext(ctx context.Context) (*Identity, bool) {
	id, ok := ctx.Value(identityCtxKey).(*Identity)
	return id, ok
}

// WithIdentity attaches an Identity to ctx. Useful for tests.
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityCtxKey, id)
}

type errBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// We deliberately don't import internal/api/middleware to avoid coupling
// (per the integration boundary). The error envelope shape matches it.
func writeErr(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errBody{Error: code, Message: message})
}

// Middleware returns an http.Handler middleware that:
//  1. Extracts Bearer token from Authorization header
//  2. Verifies the JWT
//  3. Stashes the Identity in request context
//  4. On failure, returns 401 with a clear error envelope
//
// publicPaths is a list of URL prefixes that bypass auth (e.g. "/healthz",
// "/metrics" if exposed on the same listener).
func (v *Verifier) Middleware(publicPaths []string, log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, p := range publicPaths {
				if p != "" && strings.HasPrefix(r.URL.Path, p) {
					next.ServeHTTP(w, r)
					return
				}
			}

			raw, ok := bearerToken(r)
			if !ok {
				writeErr(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
				return
			}

			id, err := v.Verify(r.Context(), raw)
			if err != nil {
				log.Info("auth reject", "err", err, "path", r.URL.Path)
				writeErr(w, http.StatusUnauthorized, "unauthorized", "invalid token")
				return
			}

			ctx := context.WithValue(r.Context(), identityCtxKey, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	tok := strings.TrimSpace(h[len(prefix):])
	if tok == "" {
		return "", false
	}
	return tok, true
}

// RequirePermission returns a middleware that 403s when the request's
// identity lacks p. Apply per-route or per-route-group via chi.Use.
func RequirePermission(roleMap RoleMap, p Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := IdentityFromContext(r.Context())
			if !ok || id == nil {
				writeErr(w, http.StatusUnauthorized, "unauthorized", "no identity in context")
				return
			}
			if !IdentityHasPermission(roleMap, id, p) {
				writeErr(w, http.StatusForbidden, "forbidden", "missing permission "+string(p))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
