// Package handlers — SMART read + dispatch endpoints.
//
// SMART has a single file because the surface is small (one read + two
// dispatch endpoints) and they share the same {name} -> /dev/<name>
// validation.
package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/smart"
	"github.com/novanas/nova-nas/internal/jobs"
)

// SmartReader is the read-only contract for SmartHandler.
type SmartReader interface {
	Get(ctx context.Context, devicePath string) (*smart.Health, error)
}

// SmartHandler handles SMART endpoints. The Mgr field is used for
// synchronous Get; Dispatcher is used for async test/enable.
type SmartHandler struct {
	Logger     *slog.Logger
	Mgr        SmartReader
	Dispatcher Dispatcher
}

// validateDeviceName enforces alphanumeric only — names like "sda" or
// "nvme0n1". The handler converts this to /dev/<name> before passing
// to the manager. This guards against shell injection via the URL.
func validateDeviceName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// Get handles GET /api/v1/disks/{name}/smart.
func (h *SmartHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !validateDeviceName(name) {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "device name must be alphanumeric")
		return
	}
	devicePath := "/dev/" + name
	health, err := h.Mgr.Get(r.Context(), devicePath)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("smart get", "device", devicePath, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to read SMART data")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, health)
}

// RunSelfTest handles POST /api/v1/disks/{name}/smart/test?type=...
func (h *SmartHandler) RunSelfTest(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !validateDeviceName(name) {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "device name must be alphanumeric")
		return
	}
	devicePath := "/dev/" + name
	testType := r.URL.Query().Get("type")
	switch testType {
	case "short", "long", "conveyance", "abort":
		// ok
	default:
		middleware.WriteError(w, http.StatusBadRequest, "bad_type",
			"type must be short|long|conveyance|abort")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSmartRunSelfTest,
		Target:    devicePath,
		Payload:   jobs.SmartRunSelfTestPayload{DevicePath: devicePath, TestType: testType},
		Command:   "smartctl -t " + testType + " " + devicePath,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "smart:test:" + devicePath,
	})
	writeDispatchResult(w, h.Logger, "smart.selftest.run", out, err)
}

// Enable handles POST /api/v1/disks/{name}/smart/enable.
func (h *SmartHandler) Enable(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !validateDeviceName(name) {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "device name must be alphanumeric")
		return
	}
	devicePath := "/dev/" + name
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSmartEnable,
		Target:    devicePath,
		Payload:   jobs.SmartEnablePayload{DevicePath: devicePath},
		Command:   "smartctl --smart=on " + devicePath,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "smart:enable:" + devicePath,
	})
	writeDispatchResult(w, h.Logger, "smart.enable", out, err)
}
