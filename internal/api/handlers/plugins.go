// Package handlers — Tier 2 plugin engine HTTP layer. The actual
// install/uninstall/upgrade orchestration lives in internal/plugins.
package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/plugins"
)

// PluginsHandler exposes /api/v1/plugins/*. When Manager is nil every
// route responds 503.
type PluginsHandler struct {
	Logger      *slog.Logger
	Manager     *plugins.Manager
	Marketplace *plugins.MarketplaceClient
	Router      *plugins.Router
	UI          *plugins.UIAssets
}

func (h *PluginsHandler) ready(w http.ResponseWriter) bool {
	if h == nil || h.Manager == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_available", "plugins subsystem not configured")
		return false
	}
	return true
}

// Index GET /plugins/index — returns the marketplace catalog.
func (h *PluginsHandler) Index(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Marketplace == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_available", "marketplace not configured")
		return
	}
	force := r.URL.Query().Get("refresh") == "true"
	idx, err := h.Marketplace.FetchIndex(r.Context(), force)
	if err != nil {
		middleware.WriteError(w, http.StatusBadGateway, "marketplace_unreachable", err.Error())
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, idx)
}

// IndexEntry GET /plugins/index/{name}.
func (h *PluginsHandler) IndexEntry(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Marketplace == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_available", "marketplace not configured")
		return
	}
	name := chi.URLParam(r, "name")
	idx, err := h.Marketplace.FetchIndex(r.Context(), false)
	if err != nil {
		middleware.WriteError(w, http.StatusBadGateway, "marketplace_unreachable", err.Error())
		return
	}
	for i := range idx.Plugins {
		if idx.Plugins[i].Name == name {
			middleware.WriteJSON(w, h.Logger, http.StatusOK, idx.Plugins[i])
			return
		}
	}
	middleware.WriteError(w, http.StatusNotFound, "not_found", "no such plugin in index")
}

// List GET /plugins — installed plugins.
func (h *PluginsHandler) List(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	out, err := h.Manager.List(r.Context())
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if out == nil {
		out = []plugins.Installation{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, out)
}

// Get GET /plugins/{name}.
func (h *PluginsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "name")
	inst, err := h.Manager.Get(r.Context(), name)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, inst)
}

// Install POST /plugins.
func (h *PluginsHandler) Install(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var req plugins.InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	inst, err := h.Manager.Install(r.Context(), req)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusCreated, inst)
}

// Upgrade PATCH /plugins/{name}.
func (h *PluginsHandler) Upgrade(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "name")
	var req struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	inst, err := h.Manager.Upgrade(r.Context(), name, req.Version)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, inst)
}

// Uninstall DELETE /plugins/{name}?purge=true.
func (h *PluginsHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "name")
	purge := r.URL.Query().Get("purge") == "true"
	if err := h.Manager.Uninstall(r.Context(), name, purge); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ServeUI is the catch-all for /plugins/{name}/ui/{path:*}.
func (h *PluginsHandler) ServeUI(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.UI == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_available", "ui server not configured")
		return
	}
	name := chi.URLParam(r, "name")
	rest := chi.URLParam(r, "*")
	h.UI.Serve(w, r, name, rest)
}

// ServeProxy is the catch-all for /plugins/{name}/api/{path:*}.
func (h *PluginsHandler) ServeProxy(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Router == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_available", "router not configured")
		return
	}
	name := chi.URLParam(r, "name")
	rest := chi.URLParam(r, "*")
	if !strings.HasPrefix(rest, "/") {
		rest = "/" + rest
	}
	h.Router.ServeProxy(w, r, name, rest)
}

func (h *PluginsHandler) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, plugins.ErrNotFound):
		middleware.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, plugins.ErrAlreadyExists):
		middleware.WriteError(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, plugins.ErrInvalid):
		middleware.WriteError(w, http.StatusBadRequest, "invalid_argument", err.Error())
	default:
		if h.Logger != nil {
			h.Logger.Warn("plugins handler", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}
