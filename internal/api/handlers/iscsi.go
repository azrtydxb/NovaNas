// Package handlers — iSCSI read endpoints.
package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/iscsi"
)

// IscsiReader is the read-only contract used by IscsiHandler. A small
// interface keeps the handler easy to fake in tests.
type IscsiReader interface {
	ListTargets(ctx context.Context) ([]iscsi.Target, error)
	GetTarget(ctx context.Context, iqn string) (*iscsi.TargetDetail, error)
}

// IscsiHandler exposes synchronous read endpoints for iSCSI targets.
type IscsiHandler struct {
	Logger *slog.Logger
	Mgr    IscsiReader
}

// ListTargets handles GET /api/v1/iscsi/targets.
func (h *IscsiHandler) ListTargets(w http.ResponseWriter, r *http.Request) {
	ts, err := h.Mgr.ListTargets(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("iscsi list", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list iSCSI targets")
		return
	}
	if ts == nil {
		ts = []iscsi.Target{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, ts)
}

// GetTarget handles GET /api/v1/iscsi/targets/{iqn}.
func (h *IscsiHandler) GetTarget(w http.ResponseWriter, r *http.Request) {
	iqn := chi.URLParam(r, "iqn")
	if iqn == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_iqn", "iqn required")
		return
	}
	d, err := h.Mgr.GetTarget(r.Context(), iqn)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("iscsi get", "iqn", iqn, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to get target")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, d)
}
