package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
)

type DatasetManager interface {
	List(ctx context.Context, root string) ([]dataset.Dataset, error)
	Get(ctx context.Context, name string) (*dataset.Detail, error)
}

type DatasetsHandler struct {
	Logger   *slog.Logger
	Datasets DatasetManager
}

// List handles GET /datasets. The query parameter is named `pool` for API
// ergonomics, but the underlying dataset.Manager.List accepts any dataset
// path as a recursion root — `?pool=tank/home` will correctly list a
// subtree.
func (h *DatasetsHandler) List(w http.ResponseWriter, r *http.Request) {
	root := r.URL.Query().Get("pool")
	ds, err := h.Datasets.List(r.Context(), root)
	if err != nil {
		h.Logger.Error("datasets list", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list datasets")
		return
	}
	if ds == nil {
		ds = []dataset.Dataset{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, ds)
}

func (h *DatasetsHandler) Get(w http.ResponseWriter, r *http.Request) {
	encoded := chi.URLParam(r, "fullname")
	name, err := url.PathUnescape(encoded)
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "invalid url-encoded name")
		return
	}
	d, err := h.Datasets.Get(r.Context(), name)
	if err != nil {
		if errors.Is(err, dataset.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "dataset not found")
			return
		}
		h.Logger.Error("datasets get", "err", err, "name", name)
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to get dataset")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, d)
}
