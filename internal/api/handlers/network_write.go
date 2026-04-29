// Package handlers — Network write (dispatch) endpoints.
//
// These handlers must perform the management-interface guard *before*
// dispatching, so they hold a direct reference to *network.Manager
// (rather than a small read interface). When the request's source IP
// can be matched to a live interface, modifications to that interface
// require force=true. When the source IP cannot be parsed or resolves
// to loopback, *all* modifications require force=true (fail closed).
package handlers

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/network"
	"github.com/novanas/nova-nas/internal/jobs"
)

// NetworkWriteHandler dispatches mutating network operations as jobs,
// gated by the management-interface guard.
type NetworkWriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
	Mgr        *network.Manager
}

// remoteIP extracts the source IP from r.RemoteAddr. Returns nil if
// parsing fails. Loopback IPs are returned (the caller decides what to
// do).
func remoteIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr might already be a bare IP (rare with stdlib but
		// some test harnesses do it).
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

// forceParam returns whether the request opts in via ?force=true.
func forceParam(r *http.Request) bool {
	v := strings.ToLower(r.URL.Query().Get("force"))
	return v == "true" || v == "1" || v == "yes"
}

// guard checks whether the request is allowed to touch ifaces named in
// touched. When force=true is set the guard is skipped. Returns true if
// the request should proceed; on failure it has already written the
// error envelope.
//
// Behaviour:
//   - source IP is loopback or unparsable → require force=true (treat
//     all interfaces as potentially management).
//   - source IP resolves to a live iface → that iface is "the management
//     iface". Refuse if any of touched matches it.
//   - source IP doesn't match any iface → conservative: require
//     force=true (we can't prove the touched ifaces are safe).
func (h *NetworkWriteHandler) guard(w http.ResponseWriter, r *http.Request, touched []string) bool {
	if forceParam(r) {
		return true
	}
	src := remoteIP(r)
	if src == nil || src.IsLoopback() {
		middleware.WriteError(w, http.StatusBadRequest, "management_interface_protected",
			"source IP cannot be verified; pass force=true to override")
		return false
	}
	mgmt, err := h.Mgr.IdentifyManagementIface(src)
	if err != nil {
		// We couldn't identify the management iface; conservatively
		// refuse. This will most often happen with proxied requests
		// where the remote IP isn't a host-local one.
		middleware.WriteError(w, http.StatusBadRequest, "management_interface_protected",
			"could not resolve management interface; pass force=true to override")
		return false
	}
	for _, name := range touched {
		if name == mgmt {
			middleware.WriteError(w, http.StatusBadRequest, "management_interface_protected",
				"refusing to modify the management interface ("+mgmt+"); pass force=true to override")
			return false
		}
	}
	return true
}

// ApplyInterface handles POST /api/v1/network/configs.
func (h *NetworkWriteHandler) ApplyInterface(w http.ResponseWriter, r *http.Request) {
	var cfg network.InterfaceConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if cfg.Name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	// Both the logical Name and the kernel iface (MatchName) need to be
	// considered "touched". MatchName may glob, but for the guard we
	// just compare strings — an explicit force=true is the right escape
	// hatch.
	if !h.guard(w, r, []string{cfg.Name, cfg.MatchName}) {
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNetworkInterfaceApply,
		Target:    cfg.Name,
		Payload:   jobs.NetworkInterfaceApplyPayload{Config: cfg},
		Command:   "networkctl reload (apply " + cfg.Name + ")",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "network:iface:" + cfg.Name,
	})
	writeDispatchResult(w, h.Logger, "network.interface.apply", out, err)
}

// DeleteInterface handles DELETE /api/v1/network/configs/{name}.
func (h *NetworkWriteHandler) DeleteInterface(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	if !h.guard(w, r, []string{name}) {
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNetworkInterfaceDelete,
		Target:    name,
		Payload:   jobs.NetworkInterfaceDeletePayload{Name: name},
		Command:   "networkctl reload (delete " + name + ")",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "network:iface:" + name,
	})
	writeDispatchResult(w, h.Logger, "network.interface.delete", out, err)
}

// ApplyVLAN handles POST /api/v1/network/vlans.
func (h *NetworkWriteHandler) ApplyVLAN(w http.ResponseWriter, r *http.Request) {
	var v network.VLAN
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if v.Name == "" || v.Parent == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_input", "name and parent required")
		return
	}
	if !h.guard(w, r, []string{v.Name, v.Parent}) {
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNetworkVLANApply,
		Target:    v.Name,
		Payload:   jobs.NetworkVLANApplyPayload{VLAN: v},
		Command:   "networkctl reload (vlan " + v.Name + ")",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "network:vlan:" + v.Name,
	})
	writeDispatchResult(w, h.Logger, "network.vlan.apply", out, err)
}

// ApplyBond handles POST /api/v1/network/bonds.
func (h *NetworkWriteHandler) ApplyBond(w http.ResponseWriter, r *http.Request) {
	var b network.Bond
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if b.Name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	if len(b.Members) == 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_members", "at least one member required")
		return
	}
	touched := append([]string{b.Name}, b.Members...)
	if !h.guard(w, r, touched) {
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNetworkBondApply,
		Target:    b.Name,
		Payload:   jobs.NetworkBondApplyPayload{Bond: b},
		Command:   "networkctl reload (bond " + b.Name + ")",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "network:bond:" + b.Name,
	})
	writeDispatchResult(w, h.Logger, "network.bond.apply", out, err)
}

// Reload handles POST /api/v1/network/reload. Always requires
// force=true because reload itself can drop the management iface.
func (h *NetworkWriteHandler) Reload(w http.ResponseWriter, r *http.Request) {
	if !forceParam(r) {
		middleware.WriteError(w, http.StatusBadRequest, "management_interface_protected",
			"network reload may drop the management interface; pass force=true to override")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNetworkReload,
		Target:    "network",
		Payload:   jobs.NetworkReloadPayload{},
		Command:   "networkctl reload",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "network:reload",
	})
	writeDispatchResult(w, h.Logger, "network.reload", out, err)
}
