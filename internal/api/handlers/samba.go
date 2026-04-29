// Package handlers — Samba read endpoints.
package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/samba"
)

// SambaReader is the read-only contract used by SambaHandler.
type SambaReader interface {
	ListShares(ctx context.Context) ([]samba.Share, error)
	GetShare(ctx context.Context, name string) (*samba.Share, error)
	ListUsers(ctx context.Context) ([]samba.User, error)
}

// SambaHandler exposes synchronous read endpoints for Samba.
type SambaHandler struct {
	Logger *slog.Logger
	Mgr    SambaReader
}

// ListShares handles GET /api/v1/samba/shares.
func (h *SambaHandler) ListShares(w http.ResponseWriter, r *http.Request) {
	xs, err := h.Mgr.ListShares(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("samba list shares", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list samba shares")
		return
	}
	if xs == nil {
		xs = []samba.Share{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, xs)
}

// GetShare handles GET /api/v1/samba/shares/{name}.
func (h *SambaHandler) GetShare(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	s, err := h.Mgr.GetShare(r.Context(), name)
	if err != nil {
		if errors.Is(err, samba.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "share not found")
			return
		}
		if h.Logger != nil {
			h.Logger.Error("samba get share", "name", name, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to get share")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, s)
}

// ListUsers handles GET /api/v1/samba/users.
func (h *SambaHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	xs, err := h.Mgr.ListUsers(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("samba list users", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list samba users")
		return
	}
	if xs == nil {
		xs = []samba.User{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, xs)
}
