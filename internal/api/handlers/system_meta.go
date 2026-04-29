// Package handlers — additional /system/* endpoints (build version,
// update channel stub) that augment the existing system handler.
//
// system.go owns /system/info, /system/time, /system/hostname,
// /system/timezone, /system/ntp, /system/reboot, /system/shutdown,
// /system/cancel-shutdown. This file adds /system/version and
// /system/updates without disturbing them.
package handlers

import (
	"log/slog"
	"net/http"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/novanas/nova-nas/internal/api/middleware"
)

// SystemVersion is the build-info envelope returned by /system/version.
//
// All fields are best-effort: when nova-api is built without ldflag
// stamping, Commit/BuildTime fall back to whatever runtime/debug
// reports.
type SystemVersion struct {
	GoVersion string `json:"goVersion"`
	Commit    string `json:"commit,omitempty"`
	BuildTime string `json:"buildTime,omitempty"`
	Module    string `json:"module,omitempty"`
	Version   string `json:"version,omitempty"`
}

// SystemUpdate is the A/B image-update state. v1 is a stub returning
// {available:false, reason:...} until the OS layer is built.
//
// Production shape (documented for the UI):
//
//	{
//	  "currentVersion": "1.2.3",
//	  "availableVersion": "1.2.4",
//	  "channel": "stable",
//	  "lastChecked": "<RFC3339>",
//	  "status": "idle"|"checking"|"downloading"|"installed-pending-reboot"
//	}
type SystemUpdate struct {
	Available        bool      `json:"available"`
	Reason           string    `json:"reason,omitempty"`
	CurrentVersion   string    `json:"currentVersion,omitempty"`
	AvailableVersion string    `json:"availableVersion,omitempty"`
	Channel          string    `json:"channel,omitempty"`
	LastChecked      time.Time `json:"lastChecked,omitempty"`
	Status           string    `json:"status,omitempty"`
}

// SystemMetaHandler serves /system/version and /system/updates. The
// fields BuildCommit/BuildTime are populated by main via ldflags; both
// optional.
type SystemMetaHandler struct {
	Logger      *slog.Logger
	BuildCommit string
	BuildTime   string

	once   sync.Once
	cached SystemVersion
}

func (h *SystemMetaHandler) buildVersion() SystemVersion {
	h.once.Do(func() {
		v := SystemVersion{
			GoVersion: runtime.Version(),
			Commit:    h.BuildCommit,
			BuildTime: h.BuildTime,
		}
		if info, ok := debug.ReadBuildInfo(); ok {
			v.Module = info.Main.Path
			if v.Version == "" {
				v.Version = info.Main.Version
			}
			if v.Commit == "" {
				for _, s := range info.Settings {
					switch s.Key {
					case "vcs.revision":
						v.Commit = s.Value
					case "vcs.time":
						if v.BuildTime == "" {
							v.BuildTime = s.Value
						}
					}
				}
			}
		}
		h.cached = v
	})
	return h.cached
}

// GetVersion handles GET /api/v1/system/version.
func (h *SystemMetaHandler) GetVersion(w http.ResponseWriter, _ *http.Request) {
	middleware.WriteJSON(w, h.Logger, http.StatusOK, h.buildVersion())
}

// GetUpdates handles GET /api/v1/system/updates. v1 stub.
func (h *SystemMetaHandler) GetUpdates(w http.ResponseWriter, _ *http.Request) {
	middleware.WriteJSON(w, h.Logger, http.StatusOK, SystemUpdate{
		Available: false,
		Reason:    "image-update-channel not configured",
		Status:    "idle",
	})
}
