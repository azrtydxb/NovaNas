// Package handlers — ProtocolShare read endpoints.
package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/protocolshare"
)

// ProtocolShareReader is the read-only contract used by ProtocolShareHandler.
type ProtocolShareReader interface {
	List(ctx context.Context) ([]protocolshare.ProtocolShare, error)
	Get(ctx context.Context, share protocolshare.ProtocolShare) (*protocolshare.Detail, error)
}

// ProtocolShareHandler exposes synchronous read endpoints for ProtocolShare.
type ProtocolShareHandler struct {
	Logger *slog.Logger
	Mgr    ProtocolShareReader
}

// List handles GET /api/v1/protocol-shares.
func (h *ProtocolShareHandler) List(w http.ResponseWriter, r *http.Request) {
	xs, err := h.Mgr.List(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("protocolshare list", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list protocol shares")
		return
	}
	if xs == nil {
		xs = []protocolshare.ProtocolShare{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, xs)
}

// Get handles GET /api/v1/protocol-shares/{name}?pool=<pool>&dataset=<dataset>.
// The package-level Get takes a ProtocolShare so we reconstruct the
// minimum-viable shape from URL + query params.
func (h *ProtocolShareHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	pool := r.URL.Query().Get("pool")
	dsName := r.URL.Query().Get("dataset")
	if pool == "" || dsName == "" {
		middleware.WriteError(w, http.StatusBadRequest, "missing_query",
			"pool and dataset query params are required")
		return
	}
	d, err := h.Mgr.Get(r.Context(), protocolshare.ProtocolShare{
		Name:        name,
		Pool:        pool,
		DatasetName: dsName,
	})
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("protocolshare get", "name", name, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to get protocol share")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, d)
}
