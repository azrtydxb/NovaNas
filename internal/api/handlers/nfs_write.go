// Package handlers — NFS write (dispatch) endpoints.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/nfs"
	"github.com/novanas/nova-nas/internal/jobs"
)

// NfsWriteHandler handles mutating NFS export operations by dispatching jobs.
type NfsWriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
}

// CreateExport handles POST /api/v1/nfs/exports.
func (h *NfsWriteHandler) CreateExport(w http.ResponseWriter, r *http.Request) {
	var e nfs.Export
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if e.Name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "export name required")
		return
	}
	if e.Path == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_path", "export path required")
		return
	}
	if len(e.Clients) == 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_clients", "at least one client rule required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNfsExportCreate,
		Target:    e.Name,
		Payload:   jobs.NfsExportCreatePayload{Export: e},
		Command:   "exportfs create " + e.Name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "nfs:export:" + e.Name,
	})
	writeDispatchResult(w, h.Logger, "nfs.export.create", out, err)
}

// UpdateExport handles PATCH /api/v1/nfs/exports/{name}. The body is the
// full Export. We treat PATCH as replace-in-place (the underlying file is a
// single line — partial update of clients is not meaningful at this layer)
// and fail the request if the URL name and the body name disagree.
func (h *NfsWriteHandler) UpdateExport(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	var e nfs.Export
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if e.Name == "" {
		e.Name = name
	} else if e.Name != name {
		middleware.WriteError(w, http.StatusBadRequest, "name_mismatch", "URL name and body name disagree")
		return
	}
	if e.Path == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_path", "export path required")
		return
	}
	if len(e.Clients) == 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_clients", "at least one client rule required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNfsExportUpdate,
		Target:    name,
		Payload:   jobs.NfsExportUpdatePayload{Export: e},
		Command:   "exportfs update " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "nfs:export:" + name,
	})
	writeDispatchResult(w, h.Logger, "nfs.export.update", out, err)
}

// DeleteExport handles DELETE /api/v1/nfs/exports/{name}.
func (h *NfsWriteHandler) DeleteExport(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNfsExportDelete,
		Target:    name,
		Payload:   jobs.NfsExportDeletePayload{Name: name},
		Command:   "exportfs delete " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "nfs:export:" + name,
	})
	writeDispatchResult(w, h.Logger, "nfs.export.delete", out, err)
}

// Reload handles POST /api/v1/nfs/reload.
func (h *NfsWriteHandler) Reload(w http.ResponseWriter, r *http.Request) {
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNfsReload,
		Target:    "nfs",
		Payload:   jobs.NfsReloadPayload{},
		Command:   "exportfs -ra",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "nfs:reload",
	})
	writeDispatchResult(w, h.Logger, "nfs.reload", out, err)
}
