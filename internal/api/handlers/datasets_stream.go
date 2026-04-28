// Synchronous streaming handlers for `zfs send` and `zfs receive`. These
// are NOT routed through the job dispatcher — the dispatcher buffers
// payloads through Redis and would defeat the point of a stream. Instead
// the manager's StreamRunner is invoked from the request goroutine and
// stdout/stdin is wired straight to the HTTP body.
package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
)

// DatasetsStreamHandler wraps the dataset Manager directly (no dispatcher)
// because zfs send/receive stream binary data and cannot be modeled as
// async jobs.
type DatasetsStreamHandler struct {
	Logger  *slog.Logger
	Dataset *dataset.Manager
}

// Send streams the result of `zfs send <snapshot> [opts]` to the
// response body. Query parameters set the SendOpts:
//
//	recursive=true        -> -R
//	raw=true              -> -w
//	compressed=true       -> -c
//	largeBlock=true       -> -L
//	embeddedData=true     -> -e
//	from=<full-or-@snap>  -> -i <from>
func (h *DatasetsStreamHandler) Send(w http.ResponseWriter, r *http.Request) {
	snap, ok := decodeAndUnescapeFullname(w, r)
	if !ok {
		return
	}
	if err := names.ValidateSnapshotName(snap); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "snapshot name is invalid")
		return
	}
	if h.Dataset == nil {
		if h.Logger != nil {
			h.Logger.Error("send: dataset manager not configured")
		}
		middleware.WriteError(w, http.StatusInternalServerError, "not_configured", "dataset manager not available")
		return
	}
	q := r.URL.Query()
	opts := dataset.SendOpts{
		Recursive:       boolQuery(q.Get("recursive")),
		Raw:             boolQuery(q.Get("raw")),
		Compressed:      boolQuery(q.Get("compressed")),
		LargeBlock:      boolQuery(q.Get("largeBlock")),
		EmbeddedData:    boolQuery(q.Get("embeddedData")),
		IncrementalFrom: q.Get("from"),
	}

	// Headers must be written before any body bytes. After this, errors
	// can no longer change the status — they're surfaced by closing the
	// connection / writing a partial stream.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	if err := h.Dataset.Send(r.Context(), snap, opts, w); err != nil {
		if h.Logger != nil {
			h.Logger.Error("zfs send", "snapshot", snap, "err", err)
		}
		// Headers are already flushed; we cannot rewrite status. The
		// best we can do is end the body — the client will see a short
		// stream and treat it as a transport error.
		return
	}
}

// Receive consumes the request body as a `zfs send` stream and pipes it
// into `zfs receive <target>`. Query parameters set the RecvOpts:
//
//	force=true         -> -F
//	resumable=true     -> -s
//	origin=<snap>      -> -o origin=<snap>
func (h *DatasetsStreamHandler) Receive(w http.ResponseWriter, r *http.Request) {
	target, ok := decodeAndUnescapeFullname(w, r)
	if !ok {
		return
	}
	if err := names.ValidateDatasetName(target); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "dataset name is invalid")
		return
	}
	if h.Dataset == nil {
		if h.Logger != nil {
			h.Logger.Error("receive: dataset manager not configured")
		}
		middleware.WriteError(w, http.StatusInternalServerError, "not_configured", "dataset manager not available")
		return
	}
	q := r.URL.Query()
	opts := dataset.RecvOpts{
		Force:          boolQuery(q.Get("force")),
		Resumable:      boolQuery(q.Get("resumable")),
		OriginSnapshot: q.Get("origin"),
	}

	if err := h.Dataset.Receive(r.Context(), target, opts, r.Body); err != nil {
		if h.Logger != nil {
			h.Logger.Error("zfs receive", "target", target, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "receive_error", "zfs receive failed")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// boolQuery accepts the same truthy values the rest of the API does
// ("true"/"1"). Empty / unrecognised → false.
func boolQuery(v string) bool {
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}
