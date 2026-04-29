// Package handlers — Samba [global] settings endpoints.
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/samba"
	"github.com/novanas/nova-nas/internal/jobs"
)

// SambaGlobalsReader is the read-only contract used by SambaGlobalsHandler.
type SambaGlobalsReader interface {
	GetGlobals(ctx context.Context) (*samba.GlobalsOpts, error)
}

// SambaGlobalsHandler exposes the synchronous GET and dispatches PUT.
type SambaGlobalsHandler struct {
	Logger     *slog.Logger
	Mgr        SambaGlobalsReader
	Dispatcher Dispatcher
}

// Get handles GET /api/v1/samba/globals.
func (h *SambaGlobalsHandler) Get(w http.ResponseWriter, r *http.Request) {
	opts, err := h.Mgr.GetGlobals(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("samba get globals", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to get samba globals")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, opts)
}

// Set handles PUT /api/v1/samba/globals.
func (h *SambaGlobalsHandler) Set(w http.ResponseWriter, r *http.Request) {
	var opts samba.GlobalsOpts
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSambaSetGlobals,
		Target:    "samba:globals",
		Payload:   jobs.SambaSetGlobalsPayload{Opts: opts},
		Command:   "samba globals set",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "samba:globals",
	})
	writeDispatchResult(w, h.Logger, "samba.globals.set", out, err)
}
