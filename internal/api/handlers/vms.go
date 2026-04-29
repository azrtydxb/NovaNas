// Package handlers — VirtualMachine + snapshot/restore + console + template endpoints.
package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/vms"
)

// VMsHandler exposes /api/v1/vms*, /vm-templates, /vm-snapshots, /vm-restores.
//
// When Mgr is nil the routes 503 — the GUI degrades gracefully on hosts
// without a working KubeVirt control plane.
type VMsHandler struct {
	Logger *slog.Logger
	Mgr    *vms.Manager
}

func (h *VMsHandler) ready(w http.ResponseWriter) bool {
	if h == nil || h.Mgr == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "vms_unavailable", "KubeVirt manager is not configured")
		return false
	}
	return true
}

// translateError maps Manager errors to HTTP statuses.
func translateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, vms.ErrNotFound):
		middleware.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, vms.ErrAlreadyExists):
		middleware.WriteError(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, vms.ErrConflict):
		middleware.WriteError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, vms.ErrInvalidRequest):
		middleware.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, vms.ErrNotImplemented):
		middleware.WriteError(w, http.StatusNotImplemented, "not_implemented", err.Error())
	default:
		middleware.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
	}
}

// List handles GET /api/v1/vms.
func (h *VMsHandler) List(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	q := r.URL.Query()
	pageSize := 0
	if s := q.Get("pageSize"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			middleware.WriteError(w, http.StatusBadRequest, "invalid_query", "pageSize must be a non-negative integer")
			return
		}
		pageSize = n
	}
	page, err := h.Mgr.List(r.Context(), vms.ListOptions{Cursor: q.Get("cursor"), PageSize: pageSize})
	if err != nil {
		translateError(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, page)
}

// Get handles GET /api/v1/vms/{namespace}/{name}.
func (h *VMsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	vm, err := h.Mgr.Get(r.Context(), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"))
	if err != nil {
		translateError(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, vm)
}

// Create handles POST /api/v1/vms.
func (h *VMsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var req vms.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	vm, err := h.Mgr.Create(r.Context(), req)
	if err != nil {
		translateError(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusCreated, vm)
}

// Patch handles PATCH /api/v1/vms/{namespace}/{name}.
func (h *VMsHandler) Patch(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var p vms.PatchRequest
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	vm, err := h.Mgr.Patch(r.Context(), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"), p)
	if err != nil {
		translateError(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, vm)
}

// Delete handles DELETE /api/v1/vms/{namespace}/{name}.
func (h *VMsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	if err := h.Mgr.Delete(r.Context(), chi.URLParam(r, "namespace"), chi.URLParam(r, "name")); err != nil {
		translateError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *VMsHandler) lifecycleAction(w http.ResponseWriter, r *http.Request, fn func(string, string) error) {
	if !h.ready(w) {
		return
	}
	if err := fn(chi.URLParam(r, "namespace"), chi.URLParam(r, "name")); err != nil {
		translateError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// Start, Stop, Restart, Pause, Unpause, Migrate.
func (h *VMsHandler) Start(w http.ResponseWriter, r *http.Request) {
	h.lifecycleAction(w, r, func(ns, n string) error { return h.Mgr.Start(r.Context(), ns, n) })
}
func (h *VMsHandler) Stop(w http.ResponseWriter, r *http.Request) {
	h.lifecycleAction(w, r, func(ns, n string) error { return h.Mgr.Stop(r.Context(), ns, n) })
}
func (h *VMsHandler) Restart(w http.ResponseWriter, r *http.Request) {
	h.lifecycleAction(w, r, func(ns, n string) error { return h.Mgr.Restart(r.Context(), ns, n) })
}
func (h *VMsHandler) Pause(w http.ResponseWriter, r *http.Request) {
	h.lifecycleAction(w, r, func(ns, n string) error { return h.Mgr.Pause(r.Context(), ns, n) })
}
func (h *VMsHandler) Unpause(w http.ResponseWriter, r *http.Request) {
	h.lifecycleAction(w, r, func(ns, n string) error { return h.Mgr.Unpause(r.Context(), ns, n) })
}
func (h *VMsHandler) Migrate(w http.ResponseWriter, r *http.Request) {
	h.lifecycleAction(w, r, func(ns, n string) error { return h.Mgr.Migrate(r.Context(), ns, n) })
}

// Console handles GET /api/v1/vms/{namespace}/{name}/console.
func (h *VMsHandler) Console(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	kind := r.URL.Query().Get("kind")
	cs, err := h.Mgr.Console(r.Context(), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"), kind)
	if err != nil {
		translateError(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, cs)
}

// Serial handles GET /api/v1/vms/{namespace}/{name}/serial.
func (h *VMsHandler) Serial(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	cs, err := h.Mgr.Console(r.Context(), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"), vms.ConsoleSerial)
	if err != nil {
		translateError(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, cs)
}

// ListTemplates handles GET /api/v1/vm-templates.
func (h *VMsHandler) ListTemplates(w http.ResponseWriter, _ *http.Request) {
	if !h.ready(w) {
		return
	}
	if h.Mgr.Templates == nil {
		middleware.WriteJSON(w, h.Logger, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, map[string]any{"items": h.Mgr.Templates.List()})
}

// snapshots ----

func (h *VMsHandler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	ns := r.URL.Query().Get("namespace")
	out, err := h.Mgr.ListSnapshots(r.Context(), ns)
	if err != nil {
		translateError(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, map[string]any{"items": out})
}

func (h *VMsHandler) CreateSnapshot(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var req vms.CreateSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	s, err := h.Mgr.CreateSnapshot(r.Context(), req)
	if err != nil {
		translateError(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusCreated, s)
}

func (h *VMsHandler) DeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	if err := h.Mgr.DeleteSnapshot(r.Context(), chi.URLParam(r, "namespace"), chi.URLParam(r, "name")); err != nil {
		translateError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// restores ----

func (h *VMsHandler) ListRestores(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	ns := r.URL.Query().Get("namespace")
	out, err := h.Mgr.ListRestores(r.Context(), ns)
	if err != nil {
		translateError(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, map[string]any{"items": out})
}

func (h *VMsHandler) CreateRestore(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var req vms.CreateRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	rs, err := h.Mgr.CreateRestore(r.Context(), req)
	if err != nil {
		translateError(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusCreated, rs)
}

func (h *VMsHandler) DeleteRestore(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	if err := h.Mgr.DeleteRestore(r.Context(), chi.URLParam(r, "namespace"), chi.URLParam(r, "name")); err != nil {
		translateError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
