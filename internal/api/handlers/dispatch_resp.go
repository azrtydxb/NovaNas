package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/jobs"
)

// Dispatcher is the interface the write handlers use to enqueue async jobs.
// Defined in the consumer package per Go convention.
type Dispatcher interface {
	Dispatch(ctx context.Context, in jobs.DispatchInput) (jobs.DispatchOutput, error)
}

// writeDispatchResult writes the standard 202+Location response for a
// successfully enqueued job, or maps a dispatch error to the appropriate
// envelope:
//   - jobs.ErrDuplicate -> 409 duplicate
//   - any other error  -> 500 dispatch_error (with err logged, not leaked)
//
// op is a short identifier for log lines (e.g. "pools.create").
func writeDispatchResult(w http.ResponseWriter, logger *slog.Logger, op string, out jobs.DispatchOutput, err error) {
	if err != nil {
		if errors.Is(err, jobs.ErrDuplicate) {
			middleware.WriteError(w, http.StatusConflict, "duplicate", "another op for this resource is already in flight")
			return
		}
		if logger != nil {
			logger.Error("dispatch", "op", op, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "dispatch_error", "failed to enqueue job")
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+out.JobID.String())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"jobId": out.JobID.String()})
}
