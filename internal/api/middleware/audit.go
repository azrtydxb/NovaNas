package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type AuditQuerier interface {
	InsertAudit(ctx context.Context, p storedb.InsertAuditParams) error
}

// Audit records every state-changing request to the audit_log table.
// GET/HEAD/OPTIONS pass through unrecorded.
//
// The request body is read up to 64 KiB and stashed back so handlers can
// re-read it. Bodies that aren't valid JSON are recorded as nil; valid
// JSON is stored verbatim. Secret redaction (`password`, `token`, etc.)
// is deferred to Plan 3.
//
// Panic recovery: register Audit BEFORE Recoverer in the chain. Then a
// panic from a handler is caught by Recoverer (which writes 500 via
// rw), and Audit's post-`next` block still runs to record the rejected
// request. Reordering breaks this guarantee.
//
// Insert failures: logged at error level via the supplied logger. The
// request is NOT failed if the audit row can't be written — silent drop
// is the right tradeoff for availability over compliance, but the log
// line ensures operators see persistent failures.
func Audit(q AuditQuerier, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			body, _ := io.ReadAll(io.LimitReader(r.Body, 64*1024))
			r.Body = io.NopCloser(bytes.NewReader(body))

			rw := &respWriter{ResponseWriter: w}
			next.ServeHTTP(rw, r)

			result := "accepted"
			if rw.status >= 400 {
				result = "rejected"
			}

			var payloadParam []byte
			if json.Valid(body) {
				// TODO(plan-3): apply RedactSecrets once that helper lands
				payloadParam = body
			}

			if err := q.InsertAudit(r.Context(), storedb.InsertAuditParams{
				Actor:     nil, // v1 has no auth; populated when auth lands
				Action:    r.Method + " " + r.URL.Path,
				Target:    r.URL.Path,
				RequestID: RequestIDOf(r.Context()),
				Payload:   payloadParam,
				Result:    result,
			}); err != nil && logger != nil {
				logger.Error("audit insert failed",
					"err", err,
					"action", r.Method+" "+r.URL.Path,
					"requestID", RequestIDOf(r.Context()),
				)
			}
		})
	}
}
