// Synchronous `zpool wait` handler. Wait blocks the caller until a named
// pool activity completes; modelling that as an async job would just
// shift the polling burden — the request goroutine is the right place.
package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

// PoolsWaitHandler wraps the pool Manager directly. Wait is synchronous
// because the underlying `zpool wait -t <activity>` blocks; running it
// inside the dispatcher would just turn polling into queue polling.
type PoolsWaitHandler struct {
	Logger *slog.Logger
	Pools  *pool.Manager
}

// Wait blocks until the requested pool activity completes or the
// timeout elapses.
//
//	?activity=resilver|scrub|trim|...
//	?timeoutSec=N (optional; 0 or unset means no extra deadline)
//
// Returns 200 on success, 504 on caller timeout, 500 on host error.
func (h *PoolsWaitHandler) Wait(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	if h.Pools == nil {
		if h.Logger != nil {
			h.Logger.Error("wait: pool manager not configured")
		}
		middleware.WriteError(w, http.StatusInternalServerError, "not_configured", "pool manager not available")
		return
	}
	activity := r.URL.Query().Get("activity")
	if activity == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_activity", "activity query parameter required")
		return
	}
	var timeout time.Duration
	if v := r.URL.Query().Get("timeoutSec"); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs < 0 {
			middleware.WriteError(w, http.StatusBadRequest, "bad_timeout", "timeoutSec must be a non-negative integer")
			return
		}
		timeout = time.Duration(secs) * time.Second
	}

	err := h.Pools.Wait(r.Context(), name, activity, timeout)
	if err != nil {
		// Distinguish caller-timeout (we cancelled the ctx via a
		// deadline) from a real host failure. Manager.Wait wraps the
		// caller ctx with timeout itself; the deadline that fires is
		// the one we set.
		if errors.Is(err, context.DeadlineExceeded) {
			middleware.WriteError(w, http.StatusGatewayTimeout, "timeout", "wait timed out before activity completed")
			return
		}
		if h.Logger != nil {
			h.Logger.Error("zpool wait", "name", name, "activity", activity, "err", err)
		}
		// Bad activity is the only validation Manager.Wait does that
		// produces a non-host error.
		middleware.WriteError(w, http.StatusInternalServerError, "wait_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}
