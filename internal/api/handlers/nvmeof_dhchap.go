// Package handlers — NVMe-oF DH-HMAC-CHAP (TP4022) authentication endpoints.
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/configfs"
	"github.com/novanas/nova-nas/internal/host/nvmeof"
	"github.com/novanas/nova-nas/internal/jobs"
)

// SetHostDHChap dispatches a write of the host's DH-CHAP configuration.
// Validation of secret content / hash / dhgroup happens in the Manager
// layer; the handler only validates the NQN and JSON shape.
func (h *NvmeofWriteHandler) SetHostDHChap(w http.ResponseWriter, r *http.Request) {
	hostNQN, ok := decodeNQN(chi.URLParam(r, "nqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nqn", "nqn is invalid")
		return
	}
	var cfg nvmeof.DHChapConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNvmeofSetHostDHChap,
		Target:    hostNQN,
		Payload:   jobs.NvmeofSetHostDHChapPayload{HostNQN: hostNQN, Config: cfg},
		Command:   "nvmet host dhchap set " + hostNQN,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "nvmeof:host:dhchap:" + hostNQN,
	})
	writeDispatchResult(w, h.Logger, "nvmeof.host.dhchap.set", out, err)
}

// ClearHostDHChap dispatches a reset of the host's DH-CHAP configuration
// to kernel defaults (no keys, hmac(sha256), null DH group).
func (h *NvmeofWriteHandler) ClearHostDHChap(w http.ResponseWriter, r *http.Request) {
	hostNQN, ok := decodeNQN(chi.URLParam(r, "nqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nqn", "nqn is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNvmeofClearHostDHChap,
		Target:    hostNQN,
		Payload:   jobs.NvmeofClearHostDHChapPayload{HostNQN: hostNQN},
		Command:   "nvmet host dhchap clear " + hostNQN,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "nvmeof:host:dhchap:" + hostNQN,
	})
	writeDispatchResult(w, h.Logger, "nvmeof.host.dhchap.clear", out, err)
}

// GetHostDHChap returns the secret-elided detail (HasKey/HasCtrlKey +
// hash/dhgroup). Raw secrets are never returned.
func (h *NvmeofHandler) GetHostDHChap(w http.ResponseWriter, r *http.Request) {
	hostNQN, ok := decodeNQN(chi.URLParam(r, "nqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nqn", "nqn is invalid")
		return
	}
	d, err := h.Mgr.GetHostDHChap(r.Context(), hostNQN)
	if err != nil {
		if errors.Is(err, configfs.ErrNotExist) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "host not found")
			return
		}
		if h.Logger != nil {
			h.Logger.Error("nvmeof get host dhchap", "host", hostNQN, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to get host dhchap")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, d)
}

// SaveConfig dispatches a job that snapshots the current nvmet configfs
// state to /etc/nova-nas/nvmet-config.json (the path the boot-time
// nova-nvmet-restore service reads). This is what makes nvmet config
// survive a reboot — nvmet itself is in-memory only.
func (h *NvmeofWriteHandler) SaveConfig(w http.ResponseWriter, r *http.Request) {
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNvmeofSaveConfig,
		Target:    "nvmeof",
		Payload:   jobs.NvmeofSaveConfigPayload{},
		Command:   "nvmeof save",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "nvmeof:saveconfig",
	})
	writeDispatchResult(w, h.Logger, "nvmeof.saveconfig", out, err)
}
