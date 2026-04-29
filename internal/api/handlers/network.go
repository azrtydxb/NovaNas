// Package handlers — Network read endpoints.
package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/network"
)

// NetworkReader is the read-only contract used by NetworkHandler.
type NetworkReader interface {
	ListInterfaces(ctx context.Context) ([]network.LiveInterface, error)
	ListConfigs(ctx context.Context) ([]network.ManagedConfig, error)
	GetConfig(ctx context.Context, name string) (*network.ManagedConfig, error)
}

// NetworkHandler exposes synchronous read endpoints.
type NetworkHandler struct {
	Logger *slog.Logger
	Mgr    NetworkReader
}

// ListInterfaces handles GET /api/v1/network/interfaces.
func (h *NetworkHandler) ListInterfaces(w http.ResponseWriter, r *http.Request) {
	xs, err := h.Mgr.ListInterfaces(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("network list interfaces", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list interfaces")
		return
	}
	if xs == nil {
		xs = []network.LiveInterface{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, xs)
}

// ListConfigs handles GET /api/v1/network/configs.
func (h *NetworkHandler) ListConfigs(w http.ResponseWriter, r *http.Request) {
	xs, err := h.Mgr.ListConfigs(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("network list configs", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list configs")
		return
	}
	if xs == nil {
		xs = []network.ManagedConfig{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, xs)
}

// GetConfig handles GET /api/v1/network/configs/{name}.
func (h *NetworkHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	c, err := h.Mgr.GetConfig(r.Context(), name)
	if err != nil {
		if errors.Is(err, network.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "config not found")
			return
		}
		if h.Logger != nil {
			h.Logger.Error("network get config", "name", name, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to get config")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, c)
}
