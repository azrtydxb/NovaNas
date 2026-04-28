package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/jobs"
)

type SnapshotsWriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
}

func (h *SnapshotsWriteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dataset   string `json:"dataset"`
		Name      string `json:"name"`
		Recursive bool   `json:"recursive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	full := body.Dataset + "@" + body.Name
	if err := names.ValidateSnapshotName(full); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "snapshot name is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSnapshotCreate,
		Target:    full,
		Payload:   jobs.SnapshotCreatePayload{Dataset: body.Dataset, ShortName: body.Name, Recursive: body.Recursive},
		Command:   "zfs snapshot " + full,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "snapshots.create", out, err)
}

func (h *SnapshotsWriteHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	encoded := chi.URLParam(r, "fullname")
	name, err := url.PathUnescape(encoded)
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "invalid url-encoded name")
		return
	}
	if err := names.ValidateSnapshotName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "snapshot name is invalid")
		return
	}
	out, derr := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSnapshotDestroy,
		Target:    name,
		Payload:   jobs.SnapshotDestroyPayload{Name: name},
		Command:   "zfs destroy " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "snapshots.destroy", out, derr)
}

func (h *SnapshotsWriteHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	encoded := chi.URLParam(r, "fullname")
	dsName, err := url.PathUnescape(encoded)
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "invalid url-encoded name")
		return
	}
	if err := names.ValidateDatasetName(dsName); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "dataset name is invalid")
		return
	}
	var body struct {
		Snapshot string `json:"snapshot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	full := dsName + "@" + body.Snapshot
	if err := names.ValidateSnapshotName(full); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "snapshot name is invalid")
		return
	}
	// Rollback is keyed on the dataset (not the snapshot) because it
	// destroys all newer snapshots and rewrites dataset state. Serialize
	// against other dataset-scoped operations.
	out, derr := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSnapshotRollback,
		Target:    full,
		Payload:   jobs.SnapshotRollbackPayload{Snapshot: full},
		Command:   "zfs rollback " + full,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:" + dsName,
	})
	writeDispatchResult(w, h.Logger, "snapshots.rollback", out, derr)
}
