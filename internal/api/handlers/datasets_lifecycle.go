package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/jobs"
)

// Rename enqueues a `zfs rename` job. The current name comes from the URL,
// the new name from the request body. Recursive (`-r`) renames child
// snapshots; it is ignored by ZFS for filesystems but accepted here.
func (h *DatasetsWriteHandler) Rename(w http.ResponseWriter, r *http.Request) {
	oldName, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	var body struct {
		NewName   string `json:"newName"`
		Recursive bool   `json:"recursive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if err := names.ValidateDatasetName(body.NewName); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "newName is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetRename,
		Target:    oldName,
		Payload:   jobs.DatasetRenamePayload{OldName: oldName, NewName: body.NewName, Recursive: body.Recursive},
		Command:   "zfs rename " + oldName + " " + body.NewName,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:" + oldName,
	})
	writeDispatchResult(w, h.Logger, "datasets.rename", out, err)
}

// Clone enqueues a `zfs clone` job. The source snapshot is the URL
// fullname (must be a snapshot, e.g. tank/home@snap), and the target
// dataset path comes from the body.
func (h *DatasetsWriteHandler) Clone(w http.ResponseWriter, r *http.Request) {
	snap, ok := decodeDatasetFullnameAsSnapshot(w, r)
	if !ok {
		return
	}
	var body struct {
		Target     string            `json:"target"`
		Properties map[string]string `json:"properties"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if err := names.ValidateDatasetName(body.Target); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "target dataset name is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetClone,
		Target:    body.Target,
		Payload:   jobs.DatasetClonePayload{Snapshot: snap, Target: body.Target, Properties: body.Properties},
		Command:   "zfs clone " + snap + " " + body.Target,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:" + body.Target,
	})
	writeDispatchResult(w, h.Logger, "datasets.clone", out, err)
}

// Promote enqueues a `zfs promote` job for a clone.
func (h *DatasetsWriteHandler) Promote(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetPromote,
		Target:    name,
		Payload:   jobs.DatasetPromotePayload{Name: name},
		Command:   "zfs promote " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:" + name,
	})
	writeDispatchResult(w, h.Logger, "datasets.promote", out, err)
}

// LoadKey enqueues a `zfs load-key` job for an encrypted dataset.
func (h *DatasetsWriteHandler) LoadKey(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	var body struct {
		Keylocation string `json:"keylocation"`
		Recursive   bool   `json:"recursive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetLoadKey,
		Target:    name,
		Payload:   jobs.DatasetLoadKeyPayload{Name: name, Keylocation: body.Keylocation, Recursive: body.Recursive},
		Command:   "zfs load-key " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:" + name,
	})
	writeDispatchResult(w, h.Logger, "datasets.load_key", out, err)
}

// UnloadKey enqueues a `zfs unload-key` job.
func (h *DatasetsWriteHandler) UnloadKey(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	var body struct {
		Recursive bool `json:"recursive"`
	}
	// Body is optional for unload-key.
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
			return
		}
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetUnloadKey,
		Target:    name,
		Payload:   jobs.DatasetUnloadKeyPayload{Name: name, Recursive: body.Recursive},
		Command:   "zfs unload-key " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:" + name,
	})
	writeDispatchResult(w, h.Logger, "datasets.unload_key", out, err)
}

// ChangeKey enqueues a `zfs change-key` job.
func (h *DatasetsWriteHandler) ChangeKey(w http.ResponseWriter, r *http.Request) {
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
		Kind:      jobs.KindDatasetChangeKey,
		Target:    name,
		Payload:   jobs.DatasetChangeKeyPayload{Name: name, Properties: body.Properties},
		Command:   "zfs change-key " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:" + name,
	})
	writeDispatchResult(w, h.Logger, "datasets.change_key", out, err)
}

// decodeDatasetFullnameAsSnapshot validates the URL fullname as a
// snapshot (must contain '@'). Used by Clone where the URL identifies a
// snapshot rather than a regular dataset.
func decodeDatasetFullnameAsSnapshot(w http.ResponseWriter, r *http.Request) (string, bool) {
	name, ok := decodeAndUnescapeFullname(w, r)
	if !ok {
		return "", false
	}
	if err := names.ValidateSnapshotName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "snapshot name is invalid")
		return "", false
	}
	return name, true
}
