// Package handlers — NFS read endpoints.
package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/nfs"
)

// NfsReader is the read-only contract used by NfsHandler. A small interface
// keeps the handler easy to fake in tests.
type NfsReader interface {
	ListExports(ctx context.Context) ([]nfs.Export, error)
	GetExport(ctx context.Context, name string) (*nfs.Export, error)
	ListActive(ctx context.Context) ([]nfs.ActiveExport, error)
}

// NfsHandler exposes synchronous read endpoints for NFS exports.
type NfsHandler struct {
	Logger *slog.Logger
	Mgr    NfsReader
}

// ListExports handles GET /api/v1/nfs/exports.
func (h *NfsHandler) ListExports(w http.ResponseWriter, r *http.Request) {
	xs, err := h.Mgr.ListExports(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("nfs list exports", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list NFS exports")
		return
	}
	if xs == nil {
		xs = []nfs.Export{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, xs)
}

// GetExport handles GET /api/v1/nfs/exports/{name}.
func (h *NfsHandler) GetExport(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	e, err := h.Mgr.GetExport(r.Context(), name)
	if err != nil {
		if errors.Is(err, nfs.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "export not found")
			return
		}
		if h.Logger != nil {
			h.Logger.Error("nfs get export", "name", name, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to get export")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, e)
}

// ListActive handles GET /api/v1/nfs/exports/active.
func (h *NfsHandler) ListActive(w http.ResponseWriter, r *http.Request) {
	xs, err := h.Mgr.ListActive(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("nfs list active", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list active exports")
		return
	}
	if xs == nil {
		xs = []nfs.ActiveExport{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, xs)
}
