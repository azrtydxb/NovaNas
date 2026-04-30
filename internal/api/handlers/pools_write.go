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

// PoolsWriteHandler handles mutating pool operations.
type PoolsWriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
	Pools      ImportableLister
}

func (h *PoolsWriteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var spec pool.CreateSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if err := names.ValidatePoolName(spec.Name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolCreate,
		Target:    spec.Name,
		Payload:   jobs.PoolCreatePayload{Name: spec.Name, Spec: spec},
		Command:   "zpool create " + spec.Name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + spec.Name,
	})
	writeDispatchResult(w, h.Logger, "pools.create", out, err)
}

func (h *PoolsWriteHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolDestroy,
		Target:    name,
		Payload:   jobs.PoolDestroyPayload{Name: name},
		Command:   "zpool destroy " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name + ":destroy",
	})
	writeDispatchResult(w, h.Logger, "pools.destroy", out, err)
}

func (h *PoolsWriteHandler) Scrub(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	var action pool.ScrubAction
	switch r.URL.Query().Get("action") {
	case "", "start":
		action = pool.ScrubStart
	case "stop":
		action = pool.ScrubStop
	default:
		middleware.WriteError(w, http.StatusBadRequest, "bad_action", "action must be 'start' or 'stop'")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolScrub,
		Target:    name,
		Payload:   jobs.PoolScrubPayload{Name: name, Action: action},
		Command:   "zpool scrub " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	writeDispatchResult(w, h.Logger, "pools.scrub", out, err)
}
