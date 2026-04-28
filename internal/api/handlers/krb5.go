// Package handlers — Kerberos read endpoints.
package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/krb5"
)

// Krb5Reader is the read-only contract used by Krb5Handler.
type Krb5Reader interface {
	GetConfig(ctx context.Context) (*krb5.Config, error)
	GetIdmapdConfig(ctx context.Context) (*krb5.IdmapdConfig, error)
	ListKeytab(ctx context.Context) ([]krb5.KeytabEntry, error)
}

// Krb5Handler exposes synchronous read endpoints for Kerberos config.
type Krb5Handler struct {
	Logger *slog.Logger
	Mgr    Krb5Reader
}

// GetConfig handles GET /api/v1/krb5/config.
func (h *Krb5Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.Mgr.GetConfig(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("krb5 get config", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to read krb5 config")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, cfg)
}

// GetIdmapd handles GET /api/v1/krb5/idmapd.
func (h *Krb5Handler) GetIdmapd(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.Mgr.GetIdmapdConfig(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("krb5 get idmapd", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to read idmapd config")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, cfg)
}

// ListKeytab handles GET /api/v1/krb5/keytab. Raw key material is never
// returned — only the parsed entries (kvno, principal, encryption).
func (h *Krb5Handler) ListKeytab(w http.ResponseWriter, r *http.Request) {
	xs, err := h.Mgr.ListKeytab(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("krb5 list keytab", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list keytab")
		return
	}
	if xs == nil {
		xs = []krb5.KeytabEntry{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, xs)
}
