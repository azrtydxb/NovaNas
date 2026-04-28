package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/api/middleware"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// SSEJobsHandler streams job state updates over Server-Sent Events.
type SSEJobsHandler struct {
	Logger *slog.Logger
	Redis  *redis.Client
	Q      JobsQ
}

// keepAliveInterval is the cadence of `: keepalive\n\n` comment frames
// sent between real events. 15s is well under typical reverse-proxy idle
// timeouts (nginx 60s, AWS ALB 60s) so the connection stays alive while
// a long job (e.g. a multi-hour scrub) makes no state transitions.
const keepAliveInterval = 15 * time.Second

// Stream subscribes to job state updates first, then emits the current
// snapshot, then tails Redis pub/sub until terminal state or client
// disconnect. Subscribing before reading the snapshot closes the race
// where the worker publishes a terminal state between snapshot and
// subscribe — any messages buffered on the channel are replayed.
func (h *SSEJobsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	pgID, ok := parseUUIDParam(r)
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", "invalid job id")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		middleware.WriteError(w, http.StatusInternalServerError, "no_flusher", "stream unsupported")
		return
	}

	idStr := pgUUIDToString(pgID)

	// Subscribe first to avoid the snapshot-vs-publish race.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	sub := h.Redis.Subscribe(ctx, "job:"+idStr+":update")
	defer sub.Close()
	ch := sub.Channel()

	// Now read the snapshot. Any state change between subscribe and now
	// will already be queued on `ch` and replayed in the loop below.
	job, err := h.Q.GetJob(r.Context(), pgID)
	if err != nil {
		if errors.Is(err, storedb.ErrNoRows) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "job not found")
			return
		}
		h.Logger.Error("sse jobs get", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", "failed to load job")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Hint to nginx to disable response buffering on this stream; harmless
	// when no proxy is in the way.
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprintf(w, "event: state\ndata: %s\n\n", job.State)
	flusher.Flush()

	if isTerminalJobState(job.State) {
		return
	}

	keepalive := time.NewTicker(keepAliveInterval)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: state\ndata: %s\n\n", msg.Payload)
			flusher.Flush()
			if isTerminalJobState(msg.Payload) {
				return
			}
		}
	}
}

func isTerminalJobState(state string) bool {
	switch state {
	case "succeeded", "failed", "cancelled", "interrupted":
		return true
	}
	return false
}
