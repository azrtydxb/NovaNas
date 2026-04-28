// Package handlers — NVMe-oF read endpoints.
package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/nvmeof"
)

// NvmeofReader is the read-only contract used by NvmeofHandler.
type NvmeofReader interface {
	ListSubsystems(ctx context.Context) ([]nvmeof.Subsystem, error)
	GetSubsystem(ctx context.Context, nqn string) (*nvmeof.SubsystemDetail, error)
	ListPorts(ctx context.Context) ([]nvmeof.Port, error)
	GetHostDHChap(ctx context.Context, hostNQN string) (nvmeof.DHChapDetail, error)
}

// NvmeofHandler exposes synchronous read endpoints for nvmet state.
type NvmeofHandler struct {
	Logger *slog.Logger
	Mgr    NvmeofReader
}

func (h *NvmeofHandler) ListSubsystems(w http.ResponseWriter, r *http.Request) {
	subs, err := h.Mgr.ListSubsystems(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("nvmeof list subsystems", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list subsystems")
		return
	}
	if subs == nil {
		subs = []nvmeof.Subsystem{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, subs)
}

func (h *NvmeofHandler) GetSubsystem(w http.ResponseWriter, r *http.Request) {
	raw := chi.URLParam(r, "nqn")
	nqn, err := url.PathUnescape(raw)
	if err != nil || nqn == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nqn", "nqn is invalid")
		return
	}
	d, err := h.Mgr.GetSubsystem(r.Context(), nqn)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("nvmeof get subsystem", "nqn", nqn, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to get subsystem")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, d)
}

func (h *NvmeofHandler) ListPorts(w http.ResponseWriter, r *http.Request) {
	ports, err := h.Mgr.ListPorts(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("nvmeof list ports", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list ports")
		return
	}
	if ports == nil {
		ports = []nvmeof.Port{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, ports)
}
