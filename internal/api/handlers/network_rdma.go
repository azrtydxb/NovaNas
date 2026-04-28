// Package handlers — RDMA adapter listing.
package handlers

import (
	"log/slog"
	"net/http"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/rdma"
)

// RDMAHandler exposes /api/v1/network/rdma.
type RDMAHandler struct {
	Logger *slog.Logger
	Lister *rdma.Lister
}

// List returns all RDMA-capable adapters (ConnectX and friends). On hosts
// with no /sys/class/infiniband, returns an empty array.
func (h *RDMAHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.Lister == nil {
		middleware.WriteJSON(w, h.Logger, http.StatusOK, []rdma.Adapter{})
		return
	}
	adapters, err := h.Lister.List(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("rdma list", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list RDMA adapters")
		return
	}
	if adapters == nil {
		adapters = []rdma.Adapter{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, adapters)
}
