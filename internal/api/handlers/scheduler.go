// Package handlers — Scheduler CRUD endpoints (sync, against the DB).
//
// Schedule and target rows are pure metadata; the scheduler's tick loop
// reads enabled rows independently. We don't dispatch through the job
// system for schedule CRUD itself.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/novanas/nova-nas/internal/api/middleware"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// SchedulerQueries is the subset of *storedb.Queries used by the
// scheduler HTTP handlers. Defined as an interface so tests can fake.
type SchedulerQueries interface {
	// snapshot schedules
	ListSnapshotSchedules(ctx context.Context) ([]storedb.SnapshotSchedule, error)
	GetSnapshotSchedule(ctx context.Context, id pgtype.UUID) (storedb.SnapshotSchedule, error)
	CreateSnapshotSchedule(ctx context.Context, arg storedb.CreateSnapshotScheduleParams) (storedb.SnapshotSchedule, error)
	UpdateSnapshotSchedule(ctx context.Context, arg storedb.UpdateSnapshotScheduleParams) (storedb.SnapshotSchedule, error)
	DeleteSnapshotSchedule(ctx context.Context, id pgtype.UUID) error
	// replication targets
	ListReplicationTargets(ctx context.Context) ([]storedb.ReplicationTarget, error)
	GetReplicationTarget(ctx context.Context, id pgtype.UUID) (storedb.ReplicationTarget, error)
	CreateReplicationTarget(ctx context.Context, arg storedb.CreateReplicationTargetParams) (storedb.ReplicationTarget, error)
	DeleteReplicationTarget(ctx context.Context, id pgtype.UUID) error
	// replication schedules
	ListReplicationSchedules(ctx context.Context) ([]storedb.ReplicationSchedule, error)
	GetReplicationSchedule(ctx context.Context, id pgtype.UUID) (storedb.ReplicationSchedule, error)
	CreateReplicationSchedule(ctx context.Context, arg storedb.CreateReplicationScheduleParams) (storedb.ReplicationSchedule, error)
	UpdateReplicationSchedule(ctx context.Context, arg storedb.UpdateReplicationScheduleParams) (storedb.ReplicationSchedule, error)
	DeleteReplicationSchedule(ctx context.Context, id pgtype.UUID) error
}

// SchedulerHandler exposes CRUD over snapshot schedules, replication
// targets, and replication schedules.
type SchedulerHandler struct {
	Logger *slog.Logger
	Q      SchedulerQueries
}

// ---------- helpers ----------

func parseUUID(w http.ResponseWriter, r *http.Request, key string) (pgtype.UUID, bool) {
	raw := chi.URLParam(r, key)
	id, err := uuid.Parse(raw)
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", "invalid id")
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: id, Valid: true}, true
}

// ---------- snapshot schedules ----------

// snapshotScheduleRequest is the create/update body shape.
type snapshotScheduleRequest struct {
	Dataset          string `json:"dataset"`
	Name             string `json:"name"`
	Cron             string `json:"cron"`
	Recursive        bool   `json:"recursive"`
	SnapshotPrefix   string `json:"snapshotPrefix"`
	RetentionHourly  int32  `json:"retentionHourly"`
	RetentionDaily   int32  `json:"retentionDaily"`
	RetentionWeekly  int32  `json:"retentionWeekly"`
	RetentionMonthly int32  `json:"retentionMonthly"`
	RetentionYearly  int32  `json:"retentionYearly"`
	Enabled          bool   `json:"enabled"`
}

// ListSnapshotSchedules handles GET /api/v1/scheduler/snapshot-schedules.
func (h *SchedulerHandler) ListSnapshotSchedules(w http.ResponseWriter, r *http.Request) {
	xs, err := h.Q.ListSnapshotSchedules(r.Context())
	if err != nil {
		h.dbErr(w, "list snapshot schedules", err)
		return
	}
	if xs == nil {
		xs = []storedb.SnapshotSchedule{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, xs)
}

// GetSnapshotSchedule handles GET /api/v1/scheduler/snapshot-schedules/{id}.
func (h *SchedulerHandler) GetSnapshotSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}
	s, err := h.Q.GetSnapshotSchedule(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "schedule not found")
			return
		}
		h.dbErr(w, "get snapshot schedule", err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, s)
}

// CreateSnapshotSchedule handles POST /api/v1/scheduler/snapshot-schedules.
func (h *SchedulerHandler) CreateSnapshotSchedule(w http.ResponseWriter, r *http.Request) {
	var req snapshotScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if req.Dataset == "" || req.Name == "" || req.Cron == "" || req.SnapshotPrefix == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_input",
			"dataset, name, cron, snapshotPrefix are required")
		return
	}
	s, err := h.Q.CreateSnapshotSchedule(r.Context(), storedb.CreateSnapshotScheduleParams{
		Dataset:          req.Dataset,
		Name:             req.Name,
		Cron:             req.Cron,
		Recursive:        req.Recursive,
		SnapshotPrefix:   req.SnapshotPrefix,
		RetentionHourly:  req.RetentionHourly,
		RetentionDaily:   req.RetentionDaily,
		RetentionWeekly:  req.RetentionWeekly,
		RetentionMonthly: req.RetentionMonthly,
		RetentionYearly:  req.RetentionYearly,
		Enabled:          req.Enabled,
	})
	if err != nil {
		h.dbErr(w, "create snapshot schedule", err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusCreated, s)
}

// UpdateSnapshotSchedule handles PATCH /api/v1/scheduler/snapshot-schedules/{id}.
// PATCH is treated as a full replace of the mutable fields (mirrors the
// existing NFS pattern).
func (h *SchedulerHandler) UpdateSnapshotSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}
	var req snapshotScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if req.Cron == "" || req.SnapshotPrefix == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_input",
			"cron and snapshotPrefix are required")
		return
	}
	s, err := h.Q.UpdateSnapshotSchedule(r.Context(), storedb.UpdateSnapshotScheduleParams{
		ID:               id,
		Cron:             req.Cron,
		Recursive:        req.Recursive,
		SnapshotPrefix:   req.SnapshotPrefix,
		RetentionHourly:  req.RetentionHourly,
		RetentionDaily:   req.RetentionDaily,
		RetentionWeekly:  req.RetentionWeekly,
		RetentionMonthly: req.RetentionMonthly,
		RetentionYearly:  req.RetentionYearly,
		Enabled:          req.Enabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "schedule not found")
			return
		}
		h.dbErr(w, "update snapshot schedule", err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, s)
}

// DeleteSnapshotSchedule handles DELETE /api/v1/scheduler/snapshot-schedules/{id}.
func (h *SchedulerHandler) DeleteSnapshotSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.Q.DeleteSnapshotSchedule(r.Context(), id); err != nil {
		h.dbErr(w, "delete snapshot schedule", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- replication targets ----------

type replicationTargetRequest struct {
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          int32  `json:"port"`
	SSHUser       string `json:"sshUser"`
	SSHKeyPath    string `json:"sshKeyPath"`
	DatasetPrefix string `json:"datasetPrefix"`
}

// ListReplicationTargets handles GET /api/v1/scheduler/replication-targets.
func (h *SchedulerHandler) ListReplicationTargets(w http.ResponseWriter, r *http.Request) {
	xs, err := h.Q.ListReplicationTargets(r.Context())
	if err != nil {
		h.dbErr(w, "list replication targets", err)
		return
	}
	if xs == nil {
		xs = []storedb.ReplicationTarget{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, xs)
}

// GetReplicationTarget handles GET /api/v1/scheduler/replication-targets/{id}.
func (h *SchedulerHandler) GetReplicationTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}
	t, err := h.Q.GetReplicationTarget(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "target not found")
			return
		}
		h.dbErr(w, "get replication target", err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, t)
}

// CreateReplicationTarget handles POST /api/v1/scheduler/replication-targets.
func (h *SchedulerHandler) CreateReplicationTarget(w http.ResponseWriter, r *http.Request) {
	var req replicationTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if req.Name == "" || req.Host == "" || req.SSHUser == "" || req.DatasetPrefix == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_input",
			"name, host, sshUser, datasetPrefix are required")
		return
	}
	if req.Port == 0 {
		req.Port = 22
	}
	t, err := h.Q.CreateReplicationTarget(r.Context(), storedb.CreateReplicationTargetParams{
		Name:          req.Name,
		Host:          req.Host,
		Port:          req.Port,
		SshUser:       req.SSHUser,
		SshKeyPath:    req.SSHKeyPath,
		DatasetPrefix: req.DatasetPrefix,
	})
	if err != nil {
		h.dbErr(w, "create replication target", err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusCreated, t)
}

// DeleteReplicationTarget handles DELETE /api/v1/scheduler/replication-targets/{id}.
func (h *SchedulerHandler) DeleteReplicationTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.Q.DeleteReplicationTarget(r.Context(), id); err != nil {
		h.dbErr(w, "delete replication target", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- replication schedules ----------

type replicationScheduleRequest struct {
	SrcDataset      string `json:"srcDataset"`
	TargetID        string `json:"targetId"`
	Cron            string `json:"cron"`
	SnapshotPrefix  string `json:"snapshotPrefix"`
	RetentionRemote int32  `json:"retentionRemote"`
	Enabled         bool   `json:"enabled"`
}

// ListReplicationSchedules handles GET /api/v1/scheduler/replication-schedules.
func (h *SchedulerHandler) ListReplicationSchedules(w http.ResponseWriter, r *http.Request) {
	xs, err := h.Q.ListReplicationSchedules(r.Context())
	if err != nil {
		h.dbErr(w, "list replication schedules", err)
		return
	}
	if xs == nil {
		xs = []storedb.ReplicationSchedule{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, xs)
}

// GetReplicationSchedule handles GET /api/v1/scheduler/replication-schedules/{id}.
func (h *SchedulerHandler) GetReplicationSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}
	s, err := h.Q.GetReplicationSchedule(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "schedule not found")
			return
		}
		h.dbErr(w, "get replication schedule", err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, s)
}

// CreateReplicationSchedule handles POST /api/v1/scheduler/replication-schedules.
func (h *SchedulerHandler) CreateReplicationSchedule(w http.ResponseWriter, r *http.Request) {
	var req replicationScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if req.SrcDataset == "" || req.Cron == "" || req.SnapshotPrefix == "" || req.TargetID == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_input",
			"srcDataset, targetId, cron, snapshotPrefix are required")
		return
	}
	tid, err := uuid.Parse(req.TargetID)
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_target_id", "invalid targetId")
		return
	}
	s, err := h.Q.CreateReplicationSchedule(r.Context(), storedb.CreateReplicationScheduleParams{
		SrcDataset:      req.SrcDataset,
		TargetID:        pgtype.UUID{Bytes: tid, Valid: true},
		Cron:            req.Cron,
		SnapshotPrefix:  req.SnapshotPrefix,
		RetentionRemote: req.RetentionRemote,
		Enabled:         req.Enabled,
	})
	if err != nil {
		h.dbErr(w, "create replication schedule", err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusCreated, s)
}

// UpdateReplicationSchedule handles PATCH /api/v1/scheduler/replication-schedules/{id}.
func (h *SchedulerHandler) UpdateReplicationSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}
	var req replicationScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if req.Cron == "" || req.SnapshotPrefix == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_input",
			"cron and snapshotPrefix are required")
		return
	}
	s, err := h.Q.UpdateReplicationSchedule(r.Context(), storedb.UpdateReplicationScheduleParams{
		ID:              id,
		Cron:            req.Cron,
		SnapshotPrefix:  req.SnapshotPrefix,
		RetentionRemote: req.RetentionRemote,
		Enabled:         req.Enabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "schedule not found")
			return
		}
		h.dbErr(w, "update replication schedule", err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, s)
}

// DeleteReplicationSchedule handles DELETE /api/v1/scheduler/replication-schedules/{id}.
func (h *SchedulerHandler) DeleteReplicationSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.Q.DeleteReplicationSchedule(r.Context(), id); err != nil {
		h.dbErr(w, "delete replication schedule", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// dbErr is a small helper that logs and returns a 500.
func (h *SchedulerHandler) dbErr(w http.ResponseWriter, op string, err error) {
	if h.Logger != nil {
		h.Logger.Error("scheduler db", "op", op, "err", err)
	}
	middleware.WriteError(w, http.StatusInternalServerError, "db_error", "database error")
}
