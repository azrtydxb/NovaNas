package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type MetadataQ interface {
	UpsertResourceMetadata(ctx context.Context, p storedb.UpsertResourceMetadataParams) (storedb.ResourceMetadatum, error)
}

type MetadataHandler struct {
	Logger *slog.Logger
	Q      MetadataQ
}

type metadataPatch struct {
	DisplayName *string         `json:"display_name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Tags        json.RawMessage `json:"tags,omitempty"`
}

func (h *MetadataHandler) patch(kind string, w http.ResponseWriter, r *http.Request, name string) {
	var body metadataPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	params := storedb.UpsertResourceMetadataParams{Kind: kind, ZfsName: name}
	if body.DisplayName != nil {
		params.DisplayName = body.DisplayName
	}
	if body.Description != nil {
		params.Description = body.Description
	}
	if len(body.Tags) > 0 {
		params.Tags = body.Tags
	}
	rec, err := h.Q.UpsertResourceMetadata(r.Context(), params)
	if err != nil {
		h.Logger.Error("metadata upsert", "kind", kind, "name", name, "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", "failed to update metadata")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, rec)
}

func (h *MetadataHandler) PoolPatch(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	h.patch("pool", w, r, name)
}

func (h *MetadataHandler) DatasetPatch(w http.ResponseWriter, r *http.Request) {
	encoded := chi.URLParam(r, "fullname")
	name, err := url.PathUnescape(encoded)
	if err != nil || names.ValidateDatasetName(name) != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "invalid dataset name")
		return
	}
	h.patch("dataset", w, r, name)
}

func (h *MetadataHandler) SnapshotPatch(w http.ResponseWriter, r *http.Request) {
	encoded := chi.URLParam(r, "fullname")
	name, err := url.PathUnescape(encoded)
	if err != nil || names.ValidateSnapshotName(name) != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "invalid snapshot name")
		return
	}
	h.patch("snapshot", w, r, name)
}
