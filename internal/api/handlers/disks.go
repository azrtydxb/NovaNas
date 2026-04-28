package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/disks"
)

type DiskLister interface {
	List(ctx context.Context) ([]disks.Disk, error)
}

type DisksHandler struct {
	Logger *slog.Logger
	Lister DiskLister
}

func (h *DisksHandler) List(w http.ResponseWriter, r *http.Request) {
	ds, err := h.Lister.List(r.Context())
	if err != nil {
		h.Logger.Error("disks list", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list disks")
		return
	}
	if ds == nil {
		ds = []disks.Disk{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, ds)
}
