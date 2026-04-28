package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/novanas/nova-nas/internal/api/middleware"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type JobsQ interface {
	GetJob(ctx context.Context, id pgtype.UUID) (storedb.Job, error)
	ListJobs(ctx context.Context, p storedb.ListJobsParams) ([]storedb.Job, error)
	CancelJob(ctx context.Context, id pgtype.UUID) error
}

type JobsHandler struct {
	Logger *slog.Logger
	Q      JobsQ
}

func parseUUIDParam(r *http.Request) (pgtype.UUID, bool) {
	raw := chi.URLParam(r, "id")
	id, err := uuid.Parse(raw)
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: id, Valid: true}, true
}

func pgUUIDToString(p pgtype.UUID) string {
	return uuid.UUID(p.Bytes).String()
}

func (h *JobsHandler) Get(w http.ResponseWriter, r *http.Request) {
	pgID, ok := parseUUIDParam(r)
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", "invalid job id")
		return
	}
	job, err := h.Q.GetJob(r.Context(), pgID)
	if err != nil {
		if errors.Is(err, storedb.ErrNoRows) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "job not found")
			return
		}
		h.Logger.Error("jobs get", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", "failed to load job")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, job)
}

func (h *JobsHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := int32(100)
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = int32(v)
	}
	offset := int32(0)
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = int32(v)
	}
	params := storedb.ListJobsParams{Limit: limit, Offset: offset}
	if state := r.URL.Query().Get("state"); state != "" {
		params.State = &state
	}
	jobs, err := h.Q.ListJobs(r.Context(), params)
	if err != nil {
		h.Logger.Error("jobs list", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", "failed to list jobs")
		return
	}
	if jobs == nil {
		jobs = []storedb.Job{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, jobs)
}

func (h *JobsHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	pgID, ok := parseUUIDParam(r)
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", "invalid job id")
		return
	}
	if err := h.Q.CancelJob(r.Context(), pgID); err != nil {
		h.Logger.Error("jobs cancel", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", "failed to cancel job")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
