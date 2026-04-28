package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/jobs"
)

// DatasetsWriteHandler handles mutating dataset operations.
type DatasetsWriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
}

func (h *DatasetsWriteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var spec dataset.CreateSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	full := spec.Parent + "/" + spec.Name
	if err := names.ValidateDatasetName(full); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "dataset name is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetCreate,
		Target:    full,
		Payload:   jobs.DatasetCreatePayload{Spec: spec},
		Command:   "zfs create " + full,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:" + full,
	})
	writeDispatchResult(w, h.Logger, "datasets.create", out, err)
}

func (h *DatasetsWriteHandler) SetProps(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	var body struct {
		Properties map[string]string `json:"properties"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if len(body.Properties) == 0 {
		middleware.WriteError(w, http.StatusBadRequest, "no_props", "properties required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetSet,
		Target:    name,
		Payload:   jobs.DatasetSetPayload{Name: name, Properties: body.Properties},
		Command:   "zfs set " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "datasets.set", out, err)
}

func (h *DatasetsWriteHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	recursive := r.URL.Query().Get("recursive") == "true"
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetDestroy,
		Target:    name,
		Payload:   jobs.DatasetDestroyPayload{Name: name, Recursive: recursive},
		Command:   "zfs destroy " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:" + name,
	})
	writeDispatchResult(w, h.Logger, "datasets.destroy", out, err)
}

func decodeDatasetFullname(w http.ResponseWriter, r *http.Request) (string, bool) {
	name, ok := decodeAndUnescapeFullname(w, r)
	if !ok {
		return "", false
	}
	if err := names.ValidateDatasetName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "dataset name is invalid")
		return "", false
	}
	return name, true
}

// decodeAndUnescapeFullname returns the URL-decoded {fullname} path
// parameter. Validation is left to the caller (callers want either
// dataset or snapshot semantics).
func decodeAndUnescapeFullname(w http.ResponseWriter, r *http.Request) (string, bool) {
	encoded := chi.URLParam(r, "fullname")
	name, err := url.PathUnescape(encoded)
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "invalid url-encoded name")
		return "", false
	}
	return name, true
}
