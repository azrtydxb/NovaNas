// Package handlers — Workloads (Apps) endpoints. The actual lifecycle
// (Helm install/upgrade/uninstall against the embedded k3s cluster) lives
// in internal/workloads. This file is the thin HTTP layer.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/auth"
	"github.com/novanas/nova-nas/internal/workloads"
)

// WorkloadsHandler exposes /api/v1/workloads/*. When Lifecycle is nil
// every handler responds 503 — operators see a clean signal that k3s
// integration is not wired (e.g. dev VM without k3s).
type WorkloadsHandler struct {
	Logger    *slog.Logger
	Lifecycle workloads.Lifecycle
}

func (h *WorkloadsHandler) ready(w http.ResponseWriter) bool {
	if h == nil || h.Lifecycle == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_available", "workloads subsystem is not configured")
		return false
	}
	return true
}

// ListIndex GET /workloads/index
func (h *WorkloadsHandler) ListIndex(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	entries, err := h.Lifecycle.IndexList(r.Context())
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if entries == nil {
		entries = []workloads.IndexEntry{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, entries)
}

// GetIndexEntry GET /workloads/index/{name}
func (h *WorkloadsHandler) GetIndexEntry(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "name")
	d, err := h.Lifecycle.IndexGet(r.Context(), name)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, d)
}

// ReloadIndex POST /workloads/index/reload
func (h *WorkloadsHandler) ReloadIndex(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	if err := h.Lifecycle.IndexReload(r.Context()); err != nil {
		h.writeErr(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, map[string]string{"status": "reloaded"})
}

// List GET /workloads
func (h *WorkloadsHandler) List(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	rels, err := h.Lifecycle.List(r.Context())
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if rels == nil {
		rels = []workloads.Release{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, rels)
}

// Get GET /workloads/{releaseName}
func (h *WorkloadsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "releaseName")
	d, err := h.Lifecycle.Get(r.Context(), name)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, d)
}

// Install POST /workloads
func (h *WorkloadsHandler) Install(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var req workloads.InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if id, ok := auth.IdentityFromContext(r.Context()); ok && id != nil {
		req.InstalledBy = id.Subject
	}
	rel, err := h.Lifecycle.Install(r.Context(), req)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusCreated, rel)
}

// Upgrade PATCH /workloads/{releaseName}
func (h *WorkloadsHandler) Upgrade(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "releaseName")
	var req workloads.UpgradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	rel, err := h.Lifecycle.Upgrade(r.Context(), name, req)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, rel)
}

// Uninstall DELETE /workloads/{releaseName}
func (h *WorkloadsHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "releaseName")
	if err := h.Lifecycle.Uninstall(r.Context(), name); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Rollback POST /workloads/{releaseName}/rollback
type rollbackBody struct {
	Revision int `json:"revision"`
}

func (h *WorkloadsHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "releaseName")
	var body rollbackBody
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
	}
	if body.Revision == 0 {
		body.Revision = 1
	}
	rel, err := h.Lifecycle.Rollback(r.Context(), name, body.Revision)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, rel)
}

// Events GET /workloads/{releaseName}/events
func (h *WorkloadsHandler) Events(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "releaseName")
	evs, err := h.Lifecycle.Events(r.Context(), name)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if evs == nil {
		evs = []workloads.Event{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, evs)
}

// Logs GET /workloads/{releaseName}/logs
func (h *WorkloadsHandler) Logs(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	name := chi.URLParam(r, "releaseName")
	q := r.URL.Query()
	req := workloads.LogRequest{
		Pod:        q.Get("pod"),
		Container:  q.Get("container"),
		Follow:     q.Get("follow") == "true" || q.Get("follow") == "1",
		Timestamps: q.Get("timestamps") == "true",
		Previous:   q.Get("previous") == "true",
	}
	if v := q.Get("tail"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			req.TailLines = n
		}
	}
	if v := q.Get("sinceSeconds"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			req.Since = time.Duration(n) * time.Second
		}
	}
	stream, err := h.Lifecycle.Logs(r.Context(), name, req)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	defer stream.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := stream.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) && h.Logger != nil {
				h.Logger.Debug("workloads logs read", "err", err)
			}
			return
		}
		if r.Context().Err() != nil {
			return
		}
	}
}

func (h *WorkloadsHandler) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workloads.ErrNotFound):
		middleware.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, workloads.ErrAlreadyExists):
		middleware.WriteError(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, workloads.ErrInvalidArgument):
		middleware.WriteError(w, http.StatusBadRequest, "invalid_argument", err.Error())
	case errors.Is(err, workloads.ErrNoCluster):
		middleware.WriteError(w, http.StatusServiceUnavailable, "no_cluster", err.Error())
	default:
		if h.Logger != nil {
			h.Logger.Warn("workloads handler", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}
