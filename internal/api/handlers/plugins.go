// Package handlers — Tier 2 plugin engine HTTP layer. The actual
// install/uninstall/upgrade orchestration lives in internal/plugins.
package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
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
//
// Optional filters:
//
//   - ?displayCategory=storage   single-valued; must be one of the 14
//     known display categories or 400 is returned
//   - ?tag=s3&tag=object         repeated; AND semantics — a plugin must
//     carry every listed tag to pass through
//
// Filters are applied server-side on the cached index so Aurora's App
// Center sidebar can keep paging cheap even on large merged catalogs.
func (h *PluginsHandler) Index(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Marketplace == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_available", "marketplace not configured")
		return
	}
	q := r.URL.Query()
	force := q.Get("refresh") == "true"

	displayCat := q.Get("displayCategory")
	if displayCat != "" && !plugins.IsValidDisplayCategory(plugins.DisplayCategory(displayCat)) {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_argument",
			"unknown displayCategory: "+displayCat)
		return
	}
	tags := q["tag"]

	idx, err := h.Marketplace.FetchIndex(r.Context(), force)
	if err != nil {
		middleware.WriteError(w, http.StatusBadGateway, "marketplace_unreachable", err.Error())
		return
	}
	if displayCat == "" && len(tags) == 0 {
		middleware.WriteJSON(w, h.Logger, http.StatusOK, idx)
		return
	}
	filtered := *idx
	filtered.Plugins = filterIndexPlugins(idx.Plugins, displayCat, tags)
	middleware.WriteJSON(w, h.Logger, http.StatusOK, &filtered)
}

// filterIndexPlugins applies displayCategory + tags filters. tags is
// AND-matched: every listed tag must be present on the plugin.
func filterIndexPlugins(in []plugins.IndexPlugin, displayCat string, tags []string) []plugins.IndexPlugin {
	out := make([]plugins.IndexPlugin, 0, len(in))
	for _, p := range in {
		if displayCat != "" && p.DisplayCategory != displayCat {
			continue
		}
		if !hasAllTags(p.Tags, tags) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func hasAllTags(have, want []string) bool {
	if len(want) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(have))
	for _, t := range have {
		set[t] = struct{}{}
	}
	for _, t := range want {
		if _, ok := set[t]; !ok {
			return false
		}
	}
	return true
}

// Categories GET /plugins/categories — returns the canonical list of
// the 14 display categories with the count of plugins in each.
//
// Zero-count entries are included so Aurora's sidebar layout doesn't
// shift as plugins are installed/removed; counts are derived from the
// merged marketplace index (the same source the Index handler reads).
func (h *PluginsHandler) Categories(w http.ResponseWriter, r *http.Request) {
	type entry struct {
		Category    string `json:"category"`
		DisplayName string `json:"displayName"`
		Count       int    `json:"count"`
	}
	out := make([]entry, 0, len(plugins.AllDisplayCategories))
	for _, c := range plugins.AllDisplayCategories {
		out = append(out, entry{
			Category:    string(c),
			DisplayName: plugins.DisplayCategoryDisplayName(c),
		})
	}
	if h == nil || h.Marketplace == nil {
		// No marketplace wired: still return the stable 14-entry list
		// with zero counts so Aurora's sidebar can render. The endpoint
		// is informational and not gated on the catalog being reachable.
		middleware.WriteJSON(w, h.Logger, http.StatusOK, out)
		return
	}
	idx, err := h.Marketplace.FetchIndex(r.Context(), false)
	if err != nil {
		// Don't fail the sidebar on a transient marketplace outage —
		// return zero counts. Aurora can refresh later.
		middleware.WriteJSON(w, h.Logger, http.StatusOK, out)
		return
	}
	for _, p := range idx.Plugins {
		for i, c := range plugins.AllDisplayCategories {
			if p.DisplayCategory == string(c) {
				out[i].Count++
				break
			}
		}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, out)
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

// Uninstall DELETE /plugins/{name}?purge=true&force=true.
//
// force=true bypasses the dependency-guard. Without it, the engine
// returns 409 + a `blockedBy` envelope listing the installed plugins
// that still depend on this one.
func (h *PluginsHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "name")
	purge := r.URL.Query().Get("purge") == "true"
	force := r.URL.Query().Get("force") == "true"
	if err := h.Manager.Uninstall(r.Context(), name, plugins.UninstallOptions{Purge: purge, Force: force}); err != nil {
		var depErr *plugins.DependentsError
		if errors.As(err, &depErr) {
			middleware.WriteJSON(w, h.Logger, http.StatusConflict, map[string]any{
				"error":     "has_dependents",
				"message":   depErr.Error(),
				"plugin":    depErr.Plugin,
				"blockedBy": depErr.BlockedBy,
			})
			return
		}
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Restart POST /plugins/{name}/restart — bounces the plugin's runtime
// unit via the wired deployer. 204 on success.
func (h *PluginsHandler) Restart(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "name")
	if err := h.Manager.Restart(r.Context(), name); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Logs GET /plugins/{name}/logs?lines=N — returns the most recent N
// journal lines for the plugin's runtime unit. lines defaults to 200,
// capped at 5000.
func (h *PluginsHandler) Logs(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "name")
	lines := 200
	if raw := r.URL.Query().Get("lines"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			middleware.WriteError(w, http.StatusBadRequest, "invalid_argument", "lines must be a positive integer")
			return
		}
		lines = n
	}
	out, err := h.Manager.Logs(r.Context(), name, lines)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if out == nil {
		out = []string{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, map[string]any{"lines": out})
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
