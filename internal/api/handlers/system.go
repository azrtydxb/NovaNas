// Package handlers — System endpoints (info read + dispatch for state
// changes).
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/system"
	"github.com/novanas/nova-nas/internal/jobs"
)

// SystemReader is the read-only contract used by SystemHandler.
type SystemReader interface {
	GetInfo(ctx context.Context) (*system.Info, error)
	GetTimeConfig(ctx context.Context) (*system.TimeConfig, error)
}

// SystemHandler exposes system info reads and dispatches system state
// changes through the job system.
type SystemHandler struct {
	Logger     *slog.Logger
	Mgr        SystemReader
	Dispatcher Dispatcher
}

// GetInfo handles GET /api/v1/system/info.
func (h *SystemHandler) GetInfo(w http.ResponseWriter, r *http.Request) {
	info, err := h.Mgr.GetInfo(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("system get info", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to read system info")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, info)
}

// GetTime handles GET /api/v1/system/time.
func (h *SystemHandler) GetTime(w http.ResponseWriter, r *http.Request) {
	tc, err := h.Mgr.GetTimeConfig(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("system get time", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to read time config")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, tc)
}

type hostnameRequest struct {
	Hostname string `json:"hostname"`
}

// SetHostname handles PUT /api/v1/system/hostname.
func (h *SystemHandler) SetHostname(w http.ResponseWriter, r *http.Request) {
	var body hostnameRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if body.Hostname == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_hostname", "hostname required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSystemSetHostname,
		Target:    body.Hostname,
		Payload:   jobs.SystemSetHostnamePayload{Hostname: body.Hostname},
		Command:   "hostnamectl set-hostname " + body.Hostname,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "system:hostname",
	})
	writeDispatchResult(w, h.Logger, "system.hostname.set", out, err)
}

type timezoneRequest struct {
	Timezone string `json:"timezone"`
}

// SetTimezone handles PUT /api/v1/system/timezone.
func (h *SystemHandler) SetTimezone(w http.ResponseWriter, r *http.Request) {
	var body timezoneRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if body.Timezone == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_timezone", "timezone required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSystemSetTimezone,
		Target:    body.Timezone,
		Payload:   jobs.SystemSetTimezonePayload{Timezone: body.Timezone},
		Command:   "timedatectl set-timezone " + body.Timezone,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "system:timezone",
	})
	writeDispatchResult(w, h.Logger, "system.timezone.set", out, err)
}

type ntpRequest struct {
	Enabled bool     `json:"enabled"`
	Servers []string `json:"servers,omitempty"`
}

// SetNTP handles PUT /api/v1/system/ntp.
func (h *SystemHandler) SetNTP(w http.ResponseWriter, r *http.Request) {
	var body ntpRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSystemSetNTP,
		Target:    "ntp",
		Payload:   jobs.SystemSetNTPPayload{Enabled: body.Enabled, Servers: body.Servers},
		Command:   "timedatectl set-ntp",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "system:ntp",
	})
	writeDispatchResult(w, h.Logger, "system.ntp.set", out, err)
}

func parseDelaySec(r *http.Request) (int, bool) {
	v := r.URL.Query().Get("delaySec")
	if v == "" {
		return 0, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// Reboot handles POST /api/v1/system/reboot.
func (h *SystemHandler) Reboot(w http.ResponseWriter, r *http.Request) {
	delay, ok := parseDelaySec(r)
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_delay", "delaySec must be a non-negative integer")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSystemReboot,
		Target:    "system",
		Payload:   jobs.SystemRebootPayload{DelaySeconds: delay},
		Command:   "systemctl reboot",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "system:reboot",
	})
	writeDispatchResult(w, h.Logger, "system.reboot", out, err)
}

// Shutdown handles POST /api/v1/system/shutdown.
func (h *SystemHandler) Shutdown(w http.ResponseWriter, r *http.Request) {
	delay, ok := parseDelaySec(r)
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_delay", "delaySec must be a non-negative integer")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSystemShutdown,
		Target:    "system",
		Payload:   jobs.SystemShutdownPayload{DelaySeconds: delay},
		Command:   "systemctl poweroff",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "system:shutdown",
	})
	writeDispatchResult(w, h.Logger, "system.shutdown", out, err)
}

// CancelShutdown handles POST /api/v1/system/cancel-shutdown.
func (h *SystemHandler) CancelShutdown(w http.ResponseWriter, r *http.Request) {
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSystemCancelShutdown,
		Target:    "system",
		Payload:   jobs.SystemCancelShutdownPayload{},
		Command:   "shutdown -c",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "system:shutdown",
	})
	writeDispatchResult(w, h.Logger, "system.cancel_shutdown", out, err)
}
