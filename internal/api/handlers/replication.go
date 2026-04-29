// Package handlers — Replication-job CRUD + run history.
//
// The transport-layer here is intentionally thin: the replication.Manager
// owns validation, persistence and per-run dispatch. This handler maps
// JSON over HTTP onto Manager methods. Credential lookup happens inside
// the Asynq worker (see internal/jobs/replication_task.go), not here.
package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/jobs"
	"github.com/novanas/nova-nas/internal/replication"
)

// ReplicationManagerAPI is the slice of *replication.Manager this handler
// uses. Defined as an interface so tests can fake.
type ReplicationManagerAPI interface {
	Create(ctx context.Context, j replication.Job) (replication.Job, error)
	Update(ctx context.Context, j replication.Job) (replication.Job, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Get(ctx context.Context, id uuid.UUID) (replication.Job, error)
	List(ctx context.Context) ([]replication.Job, error)
	Runs(ctx context.Context, jobID uuid.UUID, limit int) ([]replication.Run, error)
}

// ReplicationDispatcher is the small Asynq-side façade the /run handler
// uses. The default implementation is *jobs.Dispatcher.
type ReplicationDispatcher interface {
	Dispatch(ctx context.Context, in jobs.DispatchInput) (jobs.DispatchOutput, error)
}

// ReplicationSecretsCleaner deletes per-job secrets when a job is removed.
// Optional; nil means "skip secret cleanup".
type ReplicationSecretsCleaner interface {
	DeleteJobSecrets(ctx context.Context, jobID uuid.UUID) error
}

// ReplicationHandler exposes /api/v1/replication-jobs.
type ReplicationHandler struct {
	Logger     *slog.Logger
	Mgr        ReplicationManagerAPI
	Dispatcher ReplicationDispatcher
	Secrets    ReplicationSecretsCleaner
}

// jobRequest is the wire shape for create + update. PATCH is full-replace
// of mutable fields (matches the rest of the API).
type jobRequest struct {
	Name         string                       `json:"name"`
	Backend      replication.BackendKind      `json:"backend"`
	Direction    replication.Direction        `json:"direction"`
	Source       replication.Source           `json:"source"`
	Destination  replication.Destination      `json:"destination"`
	Schedule     string                       `json:"schedule"`
	Retention    replication.RetentionPolicy  `json:"retention"`
	Enabled      *bool                        `json:"enabled,omitempty"`
	SecretRef    string                       `json:"secretRef,omitempty"`
	LastSnapshot *string                      `json:"lastSnapshot,omitempty"`
}

// jobResponse mirrors replication.Job with a stable wire shape.
type jobResponse struct {
	ID           string                       `json:"id"`
	Name         string                       `json:"name"`
	Backend      replication.BackendKind      `json:"backend"`
	Direction    replication.Direction        `json:"direction"`
	Source       replication.Source           `json:"source"`
	Destination  replication.Destination      `json:"destination"`
	Schedule     string                       `json:"schedule"`
	Retention    replication.RetentionPolicy  `json:"retention"`
	Enabled      bool                         `json:"enabled"`
	SecretRef    string                       `json:"secretRef,omitempty"`
	LastSnapshot string                       `json:"lastSnapshot,omitempty"`
	CreatedAt    time.Time                    `json:"createdAt"`
	UpdatedAt    time.Time                    `json:"updatedAt"`
}

// runResponse mirrors replication.Run.
type runResponse struct {
	ID               string     `json:"id"`
	JobID            string     `json:"jobId"`
	StartedAt        time.Time  `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
	Outcome          string     `json:"outcome"`
	BytesTransferred int64      `json:"bytesTransferred"`
	Snapshot         string     `json:"snapshot,omitempty"`
	Error            string     `json:"error,omitempty"`
}

// jobDetailResponse adds the most-recent N runs to a single-job GET.
type jobDetailResponse struct {
	jobResponse
	Runs []runResponse `json:"runs"`
}

// runListResponse is the paginated /runs envelope.
type runListResponse struct {
	Runs       []runResponse `json:"runs"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

// List handles GET /api/v1/replication-jobs.
func (h *ReplicationHandler) List(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.Mgr.List(r.Context())
	if err != nil {
		h.internalErr(w, "list", err)
		return
	}
	out := make([]jobResponse, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, jobToResponse(j))
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, out)
}

// Get handles GET /api/v1/replication-jobs/{id}.
func (h *ReplicationHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}
	j, err := h.Mgr.Get(r.Context(), id)
	if err != nil {
		h.notFoundOr(w, "get", err)
		return
	}
	limit := parseLimit(r.URL.Query().Get("runs"), 10, 100)
	runs, err := h.Mgr.Runs(r.Context(), id, limit)
	if err != nil {
		h.internalErr(w, "runs", err)
		return
	}
	resp := jobDetailResponse{jobResponse: jobToResponse(j)}
	for _, run := range runs {
		resp.Runs = append(resp.Runs, runToResponse(run))
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, resp)
}

// Create handles POST /api/v1/replication-jobs.
func (h *ReplicationHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decodeRequest(w, r)
	if !ok {
		return
	}
	job := requestToJob(req, replication.Job{})
	created, err := h.Mgr.Create(r.Context(), job)
	if err != nil {
		h.badInputOr(w, "create", err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusCreated, jobToResponse(created))
}

// Update handles PATCH /api/v1/replication-jobs/{id}. Mutable fields are
// fully replaced from the request; unset fields fall back to the
// previously-stored value.
func (h *ReplicationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}
	existing, err := h.Mgr.Get(r.Context(), id)
	if err != nil {
		h.notFoundOr(w, "get", err)
		return
	}
	req, ok := h.decodeRequest(w, r)
	if !ok {
		return
	}
	merged := requestToJob(req, existing)
	merged.ID = id
	updated, err := h.Mgr.Update(r.Context(), merged)
	if err != nil {
		h.badInputOr(w, "update", err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, jobToResponse(updated))
}

// Delete handles DELETE /api/v1/replication-jobs/{id}.
func (h *ReplicationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}
	if err := h.Mgr.Delete(r.Context(), id); err != nil {
		if errors.Is(err, replication.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "job not found")
			return
		}
		h.internalErr(w, "delete", err)
		return
	}
	if h.Secrets != nil {
		if err := h.Secrets.DeleteJobSecrets(r.Context(), id); err != nil {
			h.Logger.Warn("replication: secret cleanup failed", "id", id.String(), "err", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Run handles POST /api/v1/replication-jobs/{id}/run. It enqueues an
// Asynq replication task and returns the dispatched job-id envelope.
func (h *ReplicationHandler) Run(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}
	if _, err := h.Mgr.Get(r.Context(), id); err != nil {
		h.notFoundOr(w, "get", err)
		return
	}
	if h.Dispatcher == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "no_dispatcher", "replication dispatcher not configured")
		return
	}
	out, err := jobs.DispatchReplication(r.Context(), h.Dispatcher, id, middleware.RequestIDOf(r.Context()), "api:run")
	writeDispatchResult(w, h.Logger, "replication.run", out, err)
}

// Runs handles GET /api/v1/replication-jobs/{id}/runs with cursor-based
// pagination. The cursor encodes (started_at, run_id) as a base64
// "RFC3339Nano|uuid" pair.
func (h *ReplicationHandler) Runs(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}
	limit := parseLimit(r.URL.Query().Get("limit"), 50, 200)
	cursor := r.URL.Query().Get("cursor")
	// Phase one keeps the API surface simple: cursor is only used to skip
	// forward; ListRuns(_, limit+1) returns one extra row to derive the
	// next cursor cheaply. Cursor decoding is best-effort: a malformed
	// cursor is treated as "first page" rather than 400, since the cursor
	// is opaque to clients.
	skipBefore := time.Time{}
	if cursor != "" {
		if t, ok := decodeRunCursor(cursor); ok {
			skipBefore = t
		}
	}

	all, err := h.Mgr.Runs(r.Context(), id, limit+1)
	if err != nil {
		h.internalErr(w, "runs", err)
		return
	}
	// Apply the "skip already-returned" cursor manager-side.
	var page []replication.Run
	for _, run := range all {
		if !skipBefore.IsZero() && !run.StartedAt.Before(skipBefore) {
			continue
		}
		page = append(page, run)
		if len(page) >= limit+1 {
			break
		}
	}
	resp := runListResponse{}
	for i, run := range page {
		if i == limit {
			resp.NextCursor = encodeRunCursor(run.StartedAt)
			break
		}
		resp.Runs = append(resp.Runs, runToResponse(run))
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, resp)
}

// ---------- helpers ----------

func (h *ReplicationHandler) parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := chi.URLParam(r, "id")
	id, err := uuid.Parse(raw)
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", "invalid job id")
		return uuid.Nil, false
	}
	return id, true
}

func (h *ReplicationHandler) decodeRequest(w http.ResponseWriter, r *http.Request) (jobRequest, bool) {
	var req jobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return req, false
	}
	return req, true
}

func (h *ReplicationHandler) internalErr(w http.ResponseWriter, op string, err error) {
	if h.Logger != nil {
		h.Logger.Error("replication handler", "op", op, "err", err)
	}
	middleware.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
}

func (h *ReplicationHandler) notFoundOr(w http.ResponseWriter, op string, err error) {
	if errors.Is(err, replication.ErrNotFound) {
		middleware.WriteError(w, http.StatusNotFound, "not_found", "job not found")
		return
	}
	h.internalErr(w, op, err)
}

func (h *ReplicationHandler) badInputOr(w http.ResponseWriter, op string, err error) {
	if errors.Is(err, replication.ErrNotFound) {
		middleware.WriteError(w, http.StatusNotFound, "not_found", "job not found")
		return
	}
	// Validation errors from Job.Validate or Backend.Validate surface as
	// plain errors; treat anything other than ErrLocked as bad-input.
	if errors.Is(err, replication.ErrLocked) {
		middleware.WriteError(w, http.StatusConflict, "locked", err.Error())
		return
	}
	if h.Logger != nil {
		h.Logger.Warn("replication handler bad input", "op", op, "err", err)
	}
	middleware.WriteError(w, http.StatusBadRequest, "bad_input", err.Error())
}

// requestToJob merges a request body onto an existing job. For Create
// the existing arg is the zero Job. The Enabled tri-state pointer is
// honoured as "leave alone if nil".
func requestToJob(req jobRequest, existing replication.Job) replication.Job {
	out := existing
	if req.Name != "" {
		out.Name = req.Name
	}
	if req.Backend != "" {
		out.Backend = req.Backend
	}
	if req.Direction != "" {
		out.Direction = req.Direction
	}
	// Source/Destination/Retention are full-replace when any field is set.
	if !isZeroSource(req.Source) {
		out.Source = req.Source
	} else if existing.ID == uuid.Nil {
		out.Source = req.Source
	}
	if !isZeroDestination(req.Destination) {
		out.Destination = req.Destination
	} else if existing.ID == uuid.Nil {
		out.Destination = req.Destination
	}
	if !req.Retention.IsZero() {
		out.Retention = req.Retention
	} else if existing.ID == uuid.Nil {
		out.Retention = req.Retention
	}
	if req.Schedule != "" || existing.ID == uuid.Nil {
		out.Schedule = req.Schedule
	}
	if req.Enabled != nil {
		out.Enabled = *req.Enabled
	} else if existing.ID == uuid.Nil {
		out.Enabled = true
	}
	if req.SecretRef != "" {
		out.SecretRef = req.SecretRef
	}
	if req.LastSnapshot != nil {
		out.LastSnapshot = *req.LastSnapshot
	}
	return out
}

func isZeroSource(s replication.Source) bool {
	return s == (replication.Source{})
}

func isZeroDestination(d replication.Destination) bool {
	return d == (replication.Destination{})
}

func jobToResponse(j replication.Job) jobResponse {
	return jobResponse{
		ID:           j.ID.String(),
		Name:         j.Name,
		Backend:      j.Backend,
		Direction:    j.Direction,
		Source:       j.Source,
		Destination:  j.Destination,
		Schedule:     j.Schedule,
		Retention:    j.Retention,
		Enabled:      j.Enabled,
		SecretRef:    j.SecretRef,
		LastSnapshot: j.LastSnapshot,
		CreatedAt:    j.CreatedAt,
		UpdatedAt:    j.UpdatedAt,
	}
}

func runToResponse(r replication.Run) runResponse {
	return runResponse{
		ID:               r.ID.String(),
		JobID:            r.JobID.String(),
		StartedAt:        r.StartedAt,
		FinishedAt:       r.FinishedAt,
		Outcome:          string(r.Outcome),
		BytesTransferred: r.BytesTransferred,
		Snapshot:         r.Snapshot,
		Error:            r.Error,
	}
}

func parseLimit(raw string, def, max int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func encodeRunCursor(t time.Time) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano)))
}

func decodeRunCursor(s string) (time.Time, bool) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, string(b))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
