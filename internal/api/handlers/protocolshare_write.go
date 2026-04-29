// Package handlers — ProtocolShare write (dispatch) endpoints.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/protocolshare"
	"github.com/novanas/nova-nas/internal/jobs"
)

// ProtocolShareWriteHandler dispatches mutating ProtocolShare ops as jobs.
type ProtocolShareWriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
}

// Create handles POST /api/v1/protocol-shares.
func (h *ProtocolShareWriteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var s protocolshare.ProtocolShare
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if s.Name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "share name required")
		return
	}
	if s.Pool == "" || s.DatasetName == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_target", "pool and datasetName required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindProtocolShareCreate,
		Target:    s.Name,
		Payload:   jobs.ProtocolShareCreatePayload{Share: s},
		Command:   "protocolshare create " + s.Name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "protocolshare:" + s.Name,
	})
	writeDispatchResult(w, h.Logger, "protocolshare.create", out, err)
}

// Update handles PATCH /api/v1/protocol-shares/{name}. Body is a full
// ProtocolShare; URL name takes precedence on mismatch.
func (h *ProtocolShareWriteHandler) Update(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	var s protocolshare.ProtocolShare
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if s.Name == "" {
		s.Name = name
	} else if s.Name != name {
		middleware.WriteError(w, http.StatusBadRequest, "name_mismatch", "URL name and body name disagree")
		return
	}
	if s.Pool == "" || s.DatasetName == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_target", "pool and datasetName required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindProtocolShareUpdate,
		Target:    name,
		Payload:   jobs.ProtocolShareUpdatePayload{Share: s},
		Command:   "protocolshare update " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "protocolshare:" + name,
	})
	writeDispatchResult(w, h.Logger, "protocolshare.update", out, err)
}

// Delete handles DELETE /api/v1/protocol-shares/{name}. When the
// optional `pool` and `dataset` query params are both supplied, the
// dispatch performs a full teardown (samba + nfs + dataset destroy);
// otherwise only the nfs + samba surfaces are removed.
func (h *ProtocolShareWriteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	pool := r.URL.Query().Get("pool")
	dsName := r.URL.Query().Get("dataset")
	// Either both or neither — partial query params are a 400 to avoid
	// accidentally degrading a "full teardown" intent into a surfaces-only
	// delete that leaves the dataset behind.
	if (pool == "") != (dsName == "") {
		middleware.WriteError(w, http.StatusBadRequest, "bad_query",
			"pool and dataset query params must be supplied together")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindProtocolShareDelete,
		Target:    name,
		Payload:   jobs.ProtocolShareDeletePayload{Name: name, Pool: pool, DatasetName: dsName},
		Command:   "protocolshare delete " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "protocolshare:" + name,
	})
	writeDispatchResult(w, h.Logger, "protocolshare.delete", out, err)
}
