package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	"github.com/novanas/nova-nas/internal/jobs"
)

// Hold enqueues a `zfs hold <tag> <snapshot>` job.
func (h *SnapshotsWriteHandler) Hold(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeSnapshotFullname(w, r)
	if !ok {
		return
	}
	var body struct {
		Tag       string `json:"tag"`
		Recursive bool   `json:"recursive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if body.Tag == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_tag", "tag required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSnapshotHold,
		Target:    name,
		Payload:   jobs.SnapshotHoldPayload{Snapshot: name, Tag: body.Tag, Recursive: body.Recursive},
		Command:   "zfs hold " + body.Tag + " " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "snapshots.hold", out, err)
}

// Release enqueues a `zfs release <tag> <snapshot>` job.
func (h *SnapshotsWriteHandler) Release(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeSnapshotFullname(w, r)
	if !ok {
		return
	}
	var body struct {
		Tag       string `json:"tag"`
		Recursive bool   `json:"recursive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if body.Tag == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_tag", "tag required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSnapshotRelease,
		Target:    name,
		Payload:   jobs.SnapshotReleasePayload{Snapshot: name, Tag: body.Tag, Recursive: body.Recursive},
		Command:   "zfs release " + body.Tag + " " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "snapshots.release", out, err)
}

// SnapshotsHoldsHandler exposes `zfs holds` synchronously — it's a fast
// read so there's no point dispatching it as a job.
type SnapshotsHoldsHandler struct {
	Logger   *slog.Logger
	Snapshot *snapshot.Manager
}

func (h *SnapshotsHoldsHandler) Holds(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeSnapshotFullname(w, r)
	if !ok {
		return
	}
	if h.Snapshot == nil {
		middleware.WriteError(w, http.StatusInternalServerError, "not_configured", "snapshot manager not available")
		return
	}
	holds, err := h.Snapshot.Holds(r.Context(), name)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("zfs holds", "snapshot", name, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "holds_error", "failed to list holds")
		return
	}
	if holds == nil {
		holds = []snapshot.Hold{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(holds)
}

// decodeSnapshotFullname extracts and validates the URL {fullname} as a
// snapshot name (must contain '@').
func decodeSnapshotFullname(w http.ResponseWriter, r *http.Request) (string, bool) {
	encoded := chi.URLParam(r, "fullname")
	name, err := url.PathUnescape(encoded)
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "invalid url-encoded name")
		return "", false
	}
	if err := names.ValidateSnapshotName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "snapshot name is invalid")
		return "", false
	}
	return name, true
}
