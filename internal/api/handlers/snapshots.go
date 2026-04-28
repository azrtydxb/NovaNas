package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
)

type SnapshotManager interface {
	List(ctx context.Context, root string) ([]snapshot.Snapshot, error)
}

type SnapshotsHandler struct {
	Logger    *slog.Logger
	Snapshots SnapshotManager
}

// List handles GET /snapshots. The query parameter is named `dataset` for
// API ergonomics, but the underlying snapshot.Manager.List accepts any
// recursion root — `?dataset=tank` will list all snapshots in pool tank.
func (h *SnapshotsHandler) List(w http.ResponseWriter, r *http.Request) {
	root := r.URL.Query().Get("dataset")
	snaps, err := h.Snapshots.List(r.Context(), root)
	if err != nil {
		h.Logger.Error("snapshots list", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list snapshots")
		return
	}
	if snaps == nil {
		snaps = []snapshot.Snapshot{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, snaps)
}
