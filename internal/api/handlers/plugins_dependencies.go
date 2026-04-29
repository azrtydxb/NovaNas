// Package handlers — plugin dependency endpoints.
//
// These handlers extend PluginsHandler with the two read-only views
// Aurora needs to render install consent + uninstall confirmation
// dialogs.
package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/plugins"
)

// DependenciesResponse is the body of GET /plugins/{name}/dependencies.
// It returns both the depth-first tree (for rendering) and the flat
// install plan (the order the engine would actually walk).
type DependenciesResponse struct {
	Tree *plugins.DependencyTreeNode `json:"tree"`
	Plan []plugins.PlanStep          `json:"plan"`
}

// DependentsResponse is the body of GET /plugins/{name}/dependents.
type DependentsResponse struct {
	Plugin     string   `json:"plugin"`
	Dependents []string `json:"dependents"`
}

// Dependencies GET /plugins/{name}/dependencies.
//
// Resolves the plugin's dependency graph WITHOUT installing anything.
// If the plugin is already installed we use the stored manifest;
// otherwise we fetch it from the marketplace. ?version=… selects a
// specific version when querying an uninstalled plugin.
func (h *PluginsHandler) Dependencies(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "name")
	version := r.URL.Query().Get("version")

	manifest, err := h.resolveManifest(r, name, version)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	resolver := h.Manager.Resolver()
	tree, err := resolver.Tree(r.Context(), manifest)
	if err != nil {
		middleware.WriteError(w, http.StatusConflict, "dependency_graph_invalid", err.Error())
		return
	}
	plan, err := resolver.Plan(r.Context(), manifest)
	if err != nil {
		middleware.WriteError(w, http.StatusConflict, "dependency_graph_invalid", err.Error())
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, DependenciesResponse{Tree: tree, Plan: plan})
}

// Dependents GET /plugins/{name}/dependents.
//
// Returns the names of installed plugins whose manifests list this
// plugin as a tier-2 dependency. The plugin itself does NOT need to
// be installed for the lookup to succeed (an uninstalled name is
// simply unreferenced and returns an empty list).
func (h *PluginsHandler) Dependents(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "name")
	dependents, err := h.Manager.DependentsOf(r.Context(), name)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if dependents == nil {
		dependents = []string{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, DependentsResponse{Plugin: name, Dependents: dependents})
}

// resolveManifest returns the manifest for a plugin: from the local
// store when installed, or from the marketplace when not.
func (h *PluginsHandler) resolveManifest(r *http.Request, name, version string) (*plugins.Plugin, error) {
	if inst, err := h.Manager.Get(r.Context(), name); err == nil && inst != nil && inst.Manifest != nil {
		// Honour ?version= override only if it actually differs from
		// what's installed. Otherwise the user wants the live tree.
		if version == "" || version == inst.Version {
			return inst.Manifest, nil
		}
	}
	return h.Manager.ManifestForPlanning(r.Context(), name, version)
}
