package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/jobs"
)

// Bookmark enqueues a `zfs bookmark <snapshot> <bookmark>` job. The URL
// {fullname} must name a snapshot (contains '@'); the body's "bookmark"
// field carries the full bookmark name (`<dataset>#<short>`).
func (h *DatasetsWriteHandler) Bookmark(w http.ResponseWriter, r *http.Request) {
	snap, ok := decodeDatasetFullnameAsSnapshot(w, r)
	if !ok {
		return
	}
	var body struct {
		Bookmark string `json:"bookmark"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if body.Bookmark == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "bookmark required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetBookmark,
		Target:    body.Bookmark,
		Payload:   jobs.DatasetBookmarkPayload{Snapshot: snap, Bookmark: body.Bookmark},
		Command:   "zfs bookmark " + snap + " " + body.Bookmark,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "datasets.bookmark", out, err)
}

// DestroyBookmark enqueues a `zfs destroy <bookmark>` job. The URL
// {fullname} is the dataset (no '@', no '#'); the body carries either the
// short name (e.g. "b1") or the full bookmark name (e.g.
// "tank/home#b1"). When the body provides only the short name we
// construct the full ref by prefixing with the dataset; when the body
// provides a full bookmark name, the dataset prefix must match the URL.
func (h *DatasetsWriteHandler) DestroyBookmark(w http.ResponseWriter, r *http.Request) {
	dsName, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	var body struct {
		Bookmark string `json:"bookmark"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if body.Bookmark == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "bookmark required")
		return
	}
	full := body.Bookmark
	if !strings.Contains(full, "#") {
		full = dsName + "#" + full
	} else {
		// Validate that the dataset prefix in the body matches the URL.
		hash := strings.IndexByte(full, '#')
		if full[:hash] != dsName {
			middleware.WriteError(w, http.StatusBadRequest, "bad_name", "bookmark dataset prefix does not match URL")
			return
		}
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetDestroyBookmark,
		Target:    full,
		Payload:   jobs.DatasetDestroyBookmarkPayload{Bookmark: full},
		Command:   "zfs destroy " + full,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "datasets.destroy_bookmark", out, err)
}

// DatasetsQueryHandler holds synchronous read-only operations against the
// dataset Manager (diff, list bookmarks). These are not dispatched as
// jobs because they're fast, idempotent reads.
type DatasetsQueryHandler struct {
	Logger  *slog.Logger
	Dataset *dataset.Manager
}

// Diff returns the result of `zfs diff -H <fromSnapshot> [<to>]`.
//
//	URL fullname is the from-snapshot (must contain '@')
//	?to=<snapshot|dataset>  optional second argument
func (h *DatasetsQueryHandler) Diff(w http.ResponseWriter, r *http.Request) {
	from, ok := decodeAndUnescapeFullname(w, r)
	if !ok {
		return
	}
	if err := names.ValidateSnapshotName(from); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "from must be a snapshot name")
		return
	}
	if h.Dataset == nil {
		middleware.WriteError(w, http.StatusInternalServerError, "not_configured", "dataset manager not available")
		return
	}
	to := r.URL.Query().Get("to")
	entries, err := h.Dataset.Diff(r.Context(), from, to)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("zfs diff", "from", from, "to", to, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "diff_error", "zfs diff failed")
		return
	}
	if entries == nil {
		entries = []dataset.DatasetDiffEntry{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(entries)
}

// ListBookmarks returns `zfs list -t bookmark -r <fullname>` output.
func (h *DatasetsQueryHandler) ListBookmarks(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	if h.Dataset == nil {
		middleware.WriteError(w, http.StatusInternalServerError, "not_configured", "dataset manager not available")
		return
	}
	bookmarks, err := h.Dataset.ListBookmarks(r.Context(), name)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("zfs list bookmarks", "root", name, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "list_error", "failed to list bookmarks")
		return
	}
	if bookmarks == nil {
		bookmarks = []dataset.Bookmark{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(bookmarks)
}
