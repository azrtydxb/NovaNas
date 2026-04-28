package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/jobs"
)

func (h *PoolsWriteHandler) Checkpoint(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolCheckpoint,
		Target:    name,
		Payload:   jobs.PoolCheckpointPayload{Name: name},
		Command:   "zpool checkpoint " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	writeDispatchResult(w, h.Logger, "pools.checkpoint", out, err)
}

func (h *PoolsWriteHandler) DiscardCheckpoint(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolDiscardCheckpoint,
		Target:    name,
		Payload:   jobs.PoolDiscardCheckpointPayload{Name: name},
		Command:   "zpool checkpoint -d " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	writeDispatchResult(w, h.Logger, "pools.discard_checkpoint", out, err)
}

// Upgrade enqueues `zpool upgrade <name>` or `zpool upgrade -a` when the
// query parameter all=true is set.
func (h *PoolsWriteHandler) Upgrade(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	all := r.URL.Query().Get("all") == "true"
	if !all {
		if err := names.ValidatePoolName(name); err != nil {
			middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
			return
		}
	}
	cmd := "zpool upgrade " + name
	if all {
		cmd = "zpool upgrade -a"
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolUpgrade,
		Target:    name,
		Payload:   jobs.PoolUpgradePayload{Name: name, All: all},
		Command:   cmd,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	writeDispatchResult(w, h.Logger, "pools.upgrade", out, err)
}

func (h *PoolsWriteHandler) Reguid(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolReguid,
		Target:    name,
		Payload:   jobs.PoolReguidPayload{Name: name},
		Command:   "zpool reguid " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	writeDispatchResult(w, h.Logger, "pools.reguid", out, err)
}

// PoolsSyncHandler runs `zpool sync` synchronously: it's a fast,
// idempotent call and queueing it would just add latency. The body is
// optional; an empty body or empty "names" list means "all pools".
type PoolsSyncHandler struct {
	Logger *slog.Logger
	Pool   *pool.Manager
}

func (h *PoolsSyncHandler) Sync(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		middleware.WriteError(w, http.StatusInternalServerError, "not_configured", "pool manager not available")
		return
	}
	var body struct {
		Names []string `json:"names"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
			return
		}
	}
	for _, n := range body.Names {
		if err := names.ValidatePoolName(n); err != nil {
			middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
			return
		}
	}
	if err := h.Pool.Sync(r.Context(), body.Names); err != nil {
		if h.Logger != nil {
			h.Logger.Error("zpool sync", "names", body.Names, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "sync_error", "zpool sync failed")
		return
	}
	w.WriteHeader(http.StatusOK)
}
