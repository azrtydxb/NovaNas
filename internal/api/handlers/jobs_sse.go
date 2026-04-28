package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/api/middleware"
)

// SSEJobsHandler streams job state updates over Server-Sent Events.
type SSEJobsHandler struct {
	Logger *slog.Logger
	Redis  *redis.Client
	Q      JobsQ
}

// Stream emits the current job state immediately, then tails Redis pub/sub
// for further state transitions until a terminal state is observed or the
// client disconnects.
func (h *SSEJobsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	pgID, ok := parseUUIDParam(r)
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", "invalid job id")
		return
	}
	job, err := h.Q.GetJob(r.Context(), pgID)
	if err != nil {
		middleware.WriteError(w, http.StatusNotFound, "not_found", "job not found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Snapshot first.
	fmt.Fprintf(w, "event: state\ndata: %s\n\n", job.State)
	flusher.Flush()

	if isTerminalJobState(job.State) {
		return
	}

	// id.Bytes is a [16]byte; render the canonical hyphenated form for the
	// channel name to match what the worker publishes.
	idStr := pgUUIDToString(pgID)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	sub := h.Redis.Subscribe(ctx, "job:"+idStr+":update")
	defer sub.Close()
	ch := sub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
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
