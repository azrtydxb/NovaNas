package handlers

import (
	"context"
	"encoding/json"
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
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ds)
}
