package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

type PoolManager interface {
	List(ctx context.Context) ([]pool.Pool, error)
	Get(ctx context.Context, name string) (*pool.Detail, error)
}

type PoolsHandler struct {
	Logger *slog.Logger
	Pools  PoolManager
}

func (h *PoolsHandler) List(w http.ResponseWriter, r *http.Request) {
	pools, err := h.Pools.List(r.Context())
	if err != nil {
		h.Logger.Error("pools list", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list pools")
		return
	}
	if pools == nil {
		pools = []pool.Pool{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, pools)
}

func (h *PoolsHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	d, err := h.Pools.Get(r.Context(), name)
	if err != nil {
		if errors.Is(err, pool.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "pool not found")
			return
		}
		h.Logger.Error("pools get", "err", err, "name", name)
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to get pool")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, d)
}
