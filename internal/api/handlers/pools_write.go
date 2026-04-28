package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/jobs"
)

// Dispatcher is the interface the write handlers use to enqueue async jobs.
type Dispatcher interface {
	Dispatch(ctx context.Context, in jobs.DispatchInput) (jobs.DispatchOutput, error)
}

// PoolsWriteHandler handles mutating pool operations.
type PoolsWriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
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
	if err != nil {
		if errors.Is(err, jobs.ErrDuplicate) {
			middleware.WriteError(w, http.StatusConflict, "duplicate", "another op for this pool is already in flight")
			return
		}
		h.Logger.Error("pools create dispatch", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "dispatch_error", "failed to enqueue job")
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+out.JobID.String())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"jobId": out.JobID.String()})
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
		UniqueKey: "pool:" + name,
	})
	if err != nil {
		if errors.Is(err, jobs.ErrDuplicate) {
			middleware.WriteError(w, http.StatusConflict, "duplicate", "another op for this pool is already in flight")
			return
		}
		h.Logger.Error("pools destroy dispatch", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "dispatch_error", "failed to enqueue job")
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+out.JobID.String())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"jobId": out.JobID.String()})
}

func (h *PoolsWriteHandler) Scrub(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	action := pool.ScrubStart
	if r.URL.Query().Get("action") == "stop" {
		action = pool.ScrubStop
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolScrub,
		Target:    name,
		Payload:   jobs.PoolScrubPayload{Name: name, Action: action},
		Command:   "zpool scrub " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	if err != nil {
		if errors.Is(err, jobs.ErrDuplicate) {
			middleware.WriteError(w, http.StatusConflict, "duplicate", "another op for this pool is already in flight")
			return
		}
		h.Logger.Error("pools scrub dispatch", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "dispatch_error", "failed to enqueue job")
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+out.JobID.String())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"jobId": out.JobID.String()})
}
