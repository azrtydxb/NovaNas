// Package handlers — Scrub policy CRUD + ad-hoc scrub trigger.
//
// Policies are pure metadata (no shell-out at write time); the
// scrubpolicy.Manager tick loop reads enabled rows independently and
// dispatches scrub jobs when due. This handler talks to the same
// storedb.Queries the manager does. We don't dispatch through the job
// system for policy CRUD itself — only for the ad-hoc trigger.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/jobs"
	"github.com/novanas/nova-nas/internal/scrubpolicy"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// ScrubPolicyQueries is the subset of *storedb.Queries this handler
// uses. Defined as an interface so tests can fake.
type ScrubPolicyQueries interface {
	ListScrubPolicies(ctx context.Context) ([]storedb.ScrubPolicy, error)
	GetScrubPolicy(ctx context.Context, id pgtype.UUID) (storedb.ScrubPolicy, error)
	CreateScrubPolicy(ctx context.Context, arg storedb.CreateScrubPolicyParams) (storedb.ScrubPolicy, error)
	UpdateScrubPolicy(ctx context.Context, arg storedb.UpdateScrubPolicyParams) (storedb.ScrubPolicy, error)
	DeleteScrubPolicy(ctx context.Context, id pgtype.UUID) error
}

// ScrubPolicyHandler exposes CRUD over scrub policies and the ad-hoc
// per-pool scrub trigger.
type ScrubPolicyHandler struct {
	Logger     *slog.Logger
	Q          ScrubPolicyQueries
	Dispatcher Dispatcher
}

// scrubPolicyRequest is the create/update body shape. We accept the
// same shape for both PATCH (full replace of mutable fields) and POST.
type scrubPolicyRequest struct {
	Name     string `json:"name"`
	Pools    string `json:"pools"`
	Cron     string `json:"cron"`
	Priority string `json:"priority"`
	Enabled  bool   `json:"enabled"`
}

// scrubPolicyResponse mirrors the DB row but renders the UUID as a hex
// string and unwraps the *string LastError to a plain string for JSON
// shape stability.
type scrubPolicyResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Pools       string `json:"pools"`
	Cron        string `json:"cron"`
	Priority    string `json:"priority"`
	Enabled     bool   `json:"enabled"`
	Builtin     bool   `json:"builtin"`
	LastFiredAt string `json:"lastFiredAt,omitempty"`
	LastError   string `json:"lastError,omitempty"`
}

func toResponse(p storedb.ScrubPolicy) scrubPolicyResponse {
	r := scrubPolicyResponse{
		ID:       scrubpolicy.UUIDString(p.ID),
		Name:     p.Name,
		Pools:    p.Pools,
		Cron:     p.Cron,
		Priority: p.Priority,
		Enabled:  p.Enabled,
		Builtin:  p.Builtin,
	}
	if p.LastFiredAt.Valid {
		r.LastFiredAt = p.LastFiredAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}
	if p.LastError != nil {
		r.LastError = *p.LastError
	}
	return r
}

// List handles GET /api/v1/scrub-policies.
func (h *ScrubPolicyHandler) List(w http.ResponseWriter, r *http.Request) {
	xs, err := h.Q.ListScrubPolicies(r.Context())
	if err != nil {
		h.dbErr(w, "list", err)
		return
	}
	out := make([]scrubPolicyResponse, 0, len(xs))
	for _, p := range xs {
		out = append(out, toResponse(p))
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, out)
}

// Get handles GET /api/v1/scrub-policies/{id}.
func (h *ScrubPolicyHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}
	p, err := h.Q.GetScrubPolicy(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "policy not found")
			return
		}
		h.dbErr(w, "get", err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, toResponse(p))
}

// Create handles POST /api/v1/scrub-policies.
func (h *ScrubPolicyHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decodeRequest(w, r)
	if !ok {
		return
	}
	if req.Name == "" || req.Cron == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_input", "name and cron are required")
		return
	}
	if err := scrubpolicy.ValidateCron(req.Cron); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_cron", err.Error())
		return
	}
	if err := scrubpolicy.ValidatePriority(req.Priority); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_priority", err.Error())
		return
	}
	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}
	pools := req.Pools
	if pools == "" {
		pools = "*"
	}
	p, err := h.Q.CreateScrubPolicy(r.Context(), storedb.CreateScrubPolicyParams{
		Name:     req.Name,
		Pools:    pools,
		Cron:     req.Cron,
		Priority: priority,
		Enabled:  req.Enabled,
		Builtin:  false,
	})
	if err != nil {
		h.dbErr(w, "create", err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusCreated, toResponse(p))
}

// Update handles PATCH /api/v1/scrub-policies/{id}.
func (h *ScrubPolicyHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}
	req, ok := h.decodeRequest(w, r)
	if !ok {
		return
	}
	if req.Cron == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_input", "cron is required")
		return
	}
	if err := scrubpolicy.ValidateCron(req.Cron); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_cron", err.Error())
		return
	}
	if err := scrubpolicy.ValidatePriority(req.Priority); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_priority", err.Error())
		return
	}
	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}
	pools := req.Pools
	if pools == "" {
		pools = "*"
	}
	p, err := h.Q.UpdateScrubPolicy(r.Context(), storedb.UpdateScrubPolicyParams{
		ID:       id,
		Pools:    pools,
		Cron:     req.Cron,
		Priority: priority,
		Enabled:  req.Enabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "policy not found")
			return
		}
		h.dbErr(w, "update", err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, toResponse(p))
}

// Delete handles DELETE /api/v1/scrub-policies/{id}.
func (h *ScrubPolicyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.Q.DeleteScrubPolicy(r.Context(), id); err != nil {
		h.dbErr(w, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TriggerPoolScrub handles POST /api/v1/pools/{name}/scrub when used as
// the scrub-policy ad-hoc trigger surface. The existing
// /api/v1/pools/{name}/scrub route in PoolsWriteHandler.Scrub already
// covers the ad-hoc path under the storage:write permission; this
// handler is a parallel implementation gated on PermScrubWrite (see
// auth.rbac.go). server.go decides which surface to mount.
func (h *ScrubPolicyHandler) TriggerPoolScrub(w http.ResponseWriter, r *http.Request) {
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
	cmd := "zpool scrub " + name
	if action == pool.ScrubStop {
		cmd = "zpool scrub -s " + name
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolScrub,
		Target:    name,
		Payload:   jobs.PoolScrubPayload{Name: name, Action: action},
		Command:   cmd,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name + ":scrub",
	})
	writeDispatchResult(w, h.Logger, "scrub.trigger", out, err)
}

// ---------- helpers ----------

func (h *ScrubPolicyHandler) decodeRequest(w http.ResponseWriter, r *http.Request) (scrubPolicyRequest, bool) {
	var req scrubPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return req, false
	}
	return req, true
}

func (h *ScrubPolicyHandler) dbErr(w http.ResponseWriter, op string, err error) {
	if h.Logger != nil {
		h.Logger.Error("scrubpolicy db", "op", op, "err", err)
	}
	middleware.WriteError(w, http.StatusInternalServerError, "db_error", "database error")
}
