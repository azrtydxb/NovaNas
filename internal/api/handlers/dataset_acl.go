// Package handlers — Dataset NFSv4 ACL endpoints.
//
// GET is synchronous (reads via the concrete *dataset.Manager). The
// mutating verbs (set/append/remove) are dispatched as async jobs.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/jobs"
)

// DatasetACLReader is the read-only contract used by DatasetACLHandler.
// The existing DatasetManager interface does not include ACL ops, so we
// define a narrow interface here. *dataset.Manager satisfies it.
type DatasetACLReader interface {
	GetACL(ctx context.Context, path string) ([]dataset.ACE, error)
}

// DatasetACLHandler exposes synchronous ACL reads and dispatches ACL writes.
type DatasetACLHandler struct {
	Logger     *slog.Logger
	Dataset    DatasetACLReader
	Dispatcher Dispatcher
}

// fullnameToPath converts a dataset fullname (pool/ds) into the host
// filesystem mount path "/<pool>/<ds>". Matches protocolshare's own
// path computation.
func fullnameToPath(fullname string) string {
	return "/" + fullname
}

// Get handles GET /api/v1/datasets/{fullname}/acl.
func (h *DatasetACLHandler) Get(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	aces, err := h.Dataset.GetACL(r.Context(), fullnameToPath(name))
	if err != nil {
		if errors.Is(err, dataset.ErrACLNotSupported) {
			middleware.WriteError(w, http.StatusBadRequest, "acl_not_supported",
				"dataset does not support NFSv4 ACLs")
			return
		}
		if h.Logger != nil {
			h.Logger.Error("dataset get acl", "name", name, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to get ACL")
		return
	}
	if aces == nil {
		aces = []dataset.ACE{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, aces)
}

// Set handles PUT /api/v1/datasets/{fullname}/acl.
// Body: {"aces":[ACE,...]}.
func (h *DatasetACLHandler) Set(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	var body struct {
		ACEs []dataset.ACE `json:"aces"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if len(body.ACEs) == 0 {
		middleware.WriteError(w, http.StatusBadRequest, "no_aces", "at least one ACE required")
		return
	}
	path := fullnameToPath(name)
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetSetACL,
		Target:    name,
		Payload:   jobs.DatasetSetACLPayload{Path: path, ACEs: body.ACEs},
		Command:   "nfs4_setfacl -S - " + path,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:acl:" + name,
	})
	writeDispatchResult(w, h.Logger, "dataset.acl.set", out, err)
}

// Append handles POST /api/v1/datasets/{fullname}/acl/append.
// Body: {"ace":ACE}.
func (h *DatasetACLHandler) Append(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	var body struct {
		ACE dataset.ACE `json:"ace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	path := fullnameToPath(name)
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetAppendACE,
		Target:    name,
		Payload:   jobs.DatasetAppendACEPayload{Path: path, ACE: body.ACE},
		Command:   "nfs4_setfacl -a <ace> " + path,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:acl:" + name,
	})
	writeDispatchResult(w, h.Logger, "dataset.acl.append", out, err)
}

// Remove handles DELETE /api/v1/datasets/{fullname}/acl/{index}.
func (h *DatasetACLHandler) Remove(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	idxStr := chi.URLParam(r, "index")
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_index", "index must be a non-negative integer")
		return
	}
	path := fullnameToPath(name)
	out, dispErr := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetRemoveACE,
		Target:    name,
		Payload:   jobs.DatasetRemoveACEPayload{Path: path, Index: idx},
		Command:   "nfs4_setfacl -x " + idxStr + " " + path,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:acl:" + name,
	})
	writeDispatchResult(w, h.Logger, "dataset.acl.remove", out, dispErr)
}
