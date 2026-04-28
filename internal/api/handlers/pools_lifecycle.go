package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/jobs"
)

// ImportableLister is the read-only portion of pool.Manager used by the
// Importable handler.  A small interface makes it easy to stub in tests.
type ImportableLister interface {
	Importable(ctx context.Context) ([]pool.ImportablePool, error)
}

// Pools field on PoolsWriteHandler provides the Importable list capability.
// (All mutating operations go through the Dispatcher.)
// The field is set in cmd/nova-api/main.go when constructing the handler.

func (h *PoolsWriteHandler) Replace(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	var body struct {
		OldDisk string `json:"oldDisk"`
		NewDisk string `json:"newDisk"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolReplace,
		Target:    name,
		Payload:   jobs.PoolReplacePayload{Name: name, OldDisk: body.OldDisk, NewDisk: body.NewDisk},
		Command:   "zpool replace " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	writeDispatchResult(w, h.Logger, "pools.replace", out, err)
}

func (h *PoolsWriteHandler) Offline(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	var body struct {
		Disk      string `json:"disk"`
		Temporary bool   `json:"temporary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolOffline,
		Target:    name,
		Payload:   jobs.PoolOfflinePayload{Name: name, Disk: body.Disk, Temporary: body.Temporary},
		Command:   "zpool offline " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	writeDispatchResult(w, h.Logger, "pools.offline", out, err)
}

func (h *PoolsWriteHandler) Online(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	var body struct {
		Disk string `json:"disk"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolOnline,
		Target:    name,
		Payload:   jobs.PoolOnlinePayload{Name: name, Disk: body.Disk},
		Command:   "zpool online " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	writeDispatchResult(w, h.Logger, "pools.online", out, err)
}

func (h *PoolsWriteHandler) Clear(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	var body struct {
		Disk string `json:"disk"` // optional
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolClear,
		Target:    name,
		Payload:   jobs.PoolClearPayload{Name: name, Disk: body.Disk},
		Command:   "zpool clear " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	writeDispatchResult(w, h.Logger, "pools.clear", out, err)
}

func (h *PoolsWriteHandler) Attach(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	var body struct {
		Existing string `json:"existing"`
		NewDisk  string `json:"newDisk"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolAttach,
		Target:    name,
		Payload:   jobs.PoolAttachPayload{Name: name, Existing: body.Existing, NewDisk: body.NewDisk},
		Command:   "zpool attach " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	writeDispatchResult(w, h.Logger, "pools.attach", out, err)
}

func (h *PoolsWriteHandler) Detach(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	var body struct {
		Disk string `json:"disk"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolDetach,
		Target:    name,
		Payload:   jobs.PoolDetachPayload{Name: name, Disk: body.Disk},
		Command:   "zpool detach " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	writeDispatchResult(w, h.Logger, "pools.detach", out, err)
}

func (h *PoolsWriteHandler) Add(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	var spec pool.AddSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolAdd,
		Target:    name,
		Payload:   jobs.PoolAddPayload{Name: name, Spec: spec},
		Command:   "zpool add " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	writeDispatchResult(w, h.Logger, "pools.add", out, err)
}

func (h *PoolsWriteHandler) Export(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	var body struct {
		Force bool `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolExport,
		Target:    name,
		Payload:   jobs.PoolExportPayload{Name: name, Force: body.Force},
		Command:   "zpool export " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	writeDispatchResult(w, h.Logger, "pools.export", out, err)
}

// Import enqueues an async import job. The pool name is in the request body
// (not the URL) because the pool is not yet imported.
func (h *PoolsWriteHandler) Import(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if err := names.ValidatePoolName(body.Name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "pool name is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolImport,
		Target:    body.Name,
		Payload:   jobs.PoolImportPayload{Name: body.Name},
		Command:   "zpool import " + body.Name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + body.Name,
	})
	writeDispatchResult(w, h.Logger, "pools.import", out, err)
}

// Importable returns the list of pools available for import synchronously.
// It calls the pool Manager directly — no job is enqueued.
func (h *PoolsWriteHandler) Importable(w http.ResponseWriter, r *http.Request) {
	if h.Pools == nil {
		if h.Logger != nil {
			h.Logger.Error("importable: pool manager not configured")
		}
		middleware.WriteError(w, http.StatusInternalServerError, "not_configured", "pool manager not available")
		return
	}
	pools, err := h.Pools.Importable(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("importable", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "list_error", "failed to list importable pools")
		return
	}
	if pools == nil {
		pools = []pool.ImportablePool{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(pools)
}

