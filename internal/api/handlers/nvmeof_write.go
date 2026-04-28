// Package handlers — NVMe-oF write (dispatch) endpoints.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/nvmeof"
	"github.com/novanas/nova-nas/internal/jobs"
)

// NvmeofWriteHandler dispatches nvmet mutations.
type NvmeofWriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
}

var nqnHandlerRegexp = regexp.MustCompile(`^nqn\.[A-Za-z0-9._:][A-Za-z0-9._:\-]*$`)

func validateNQNHandler(nqn string) bool {
	if nqn == "" || len(nqn) > 223 {
		return false
	}
	if !strings.HasPrefix(nqn, "nqn.") || strings.HasPrefix(nqn, "nqn.-") {
		return false
	}
	return nqnHandlerRegexp.MatchString(nqn)
}

func decodeNQN(raw string) (string, bool) {
	s, err := url.PathUnescape(raw)
	if err != nil {
		return "", false
	}
	return s, validateNQNHandler(s)
}

func validateNVMETransport(t string) bool {
	return t == "tcp" || t == "rdma"
}

// ---------- subsystem ----------

func (h *NvmeofWriteHandler) CreateSubsystem(w http.ResponseWriter, r *http.Request) {
	var sub nvmeof.Subsystem
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if !validateNQNHandler(sub.NQN) {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nqn", "nqn is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNvmeofSubsystemCreate,
		Target:    sub.NQN,
		Payload:   jobs.NvmeofSubsystemCreatePayload{Subsystem: sub},
		Command:   "nvmet subsystem create " + sub.NQN,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "nvmeof:subsystem:" + sub.NQN,
	})
	writeDispatchResult(w, h.Logger, "nvmeof.subsystem.create", out, err)
}

func (h *NvmeofWriteHandler) DestroySubsystem(w http.ResponseWriter, r *http.Request) {
	nqn, ok := decodeNQN(chi.URLParam(r, "nqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nqn", "nqn is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNvmeofSubsystemDestroy,
		Target:    nqn,
		Payload:   jobs.NvmeofSubsystemDestroyPayload{NQN: nqn},
		Command:   "nvmet subsystem destroy " + nqn,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "nvmeof:subsystem:" + nqn,
	})
	writeDispatchResult(w, h.Logger, "nvmeof.subsystem.destroy", out, err)
}

// ---------- namespace ----------

func (h *NvmeofWriteHandler) AddNamespace(w http.ResponseWriter, r *http.Request) {
	nqn, ok := decodeNQN(chi.URLParam(r, "nqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nqn", "nqn is invalid")
		return
	}
	var ns nvmeof.Namespace
	if err := json.NewDecoder(r.Body).Decode(&ns); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if ns.NSID <= 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nsid", "nsid must be > 0")
		return
	}
	if !strings.HasPrefix(ns.DevicePath, "/dev/") {
		middleware.WriteError(w, http.StatusBadRequest, "bad_device_path", "device_path must start with /dev/")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNvmeofNamespaceAdd,
		Target:    nqn,
		Payload:   jobs.NvmeofNamespaceAddPayload{NQN: nqn, Namespace: ns},
		Command:   "nvmet namespace add " + nqn,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "nvmeof.namespace.add", out, err)
}

func (h *NvmeofWriteHandler) RemoveNamespace(w http.ResponseWriter, r *http.Request) {
	nqn, ok := decodeNQN(chi.URLParam(r, "nqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nqn", "nqn is invalid")
		return
	}
	nsid, err := strconv.Atoi(chi.URLParam(r, "nsid"))
	if err != nil || nsid <= 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nsid", "nsid must be > 0")
		return
	}
	out, derr := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNvmeofNamespaceRemove,
		Target:    nqn,
		Payload:   jobs.NvmeofNamespaceRemovePayload{NQN: nqn, NSID: nsid},
		Command:   "nvmet namespace remove " + nqn,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "nvmeof.namespace.remove", out, derr)
}

// ---------- host ----------

type nvmeofHostReq struct {
	HostNQN string `json:"hostNqn"`
}

func (h *NvmeofWriteHandler) AllowHost(w http.ResponseWriter, r *http.Request) {
	nqn, ok := decodeNQN(chi.URLParam(r, "nqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nqn", "nqn is invalid")
		return
	}
	var body nvmeofHostReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if !validateNQNHandler(body.HostNQN) {
		middleware.WriteError(w, http.StatusBadRequest, "bad_host_nqn", "hostNqn is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNvmeofHostAllow,
		Target:    nqn,
		Payload:   jobs.NvmeofHostAllowPayload{NQN: nqn, HostNQN: body.HostNQN},
		Command:   "nvmet host allow " + nqn + " " + body.HostNQN,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "nvmeof.host.allow", out, err)
}

func (h *NvmeofWriteHandler) DisallowHost(w http.ResponseWriter, r *http.Request) {
	nqn, ok := decodeNQN(chi.URLParam(r, "nqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nqn", "nqn is invalid")
		return
	}
	hostNQN, ok := decodeNQN(chi.URLParam(r, "hostNqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_host_nqn", "hostNqn is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNvmeofHostDisallow,
		Target:    nqn,
		Payload:   jobs.NvmeofHostDisallowPayload{NQN: nqn, HostNQN: hostNQN},
		Command:   "nvmet host disallow " + nqn + " " + hostNQN,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "nvmeof.host.disallow", out, err)
}

// ---------- port ----------

func (h *NvmeofWriteHandler) CreatePort(w http.ResponseWriter, r *http.Request) {
	var p nvmeof.Port
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if p.ID < 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_port_id", "port id must be >= 0")
		return
	}
	if net.ParseIP(p.IP) == nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_ip", "ip is invalid")
		return
	}
	if p.Port < 1 || p.Port > 65535 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_port", "port out of range")
		return
	}
	if !validateNVMETransport(p.Transport) {
		middleware.WriteError(w, http.StatusBadRequest, "bad_transport", "transport must be tcp or rdma")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNvmeofPortCreate,
		Target:    strconv.Itoa(p.ID),
		Payload:   jobs.NvmeofPortCreatePayload{Port: p},
		Command:   "nvmet port create " + strconv.Itoa(p.ID),
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "nvmeof:port:" + strconv.Itoa(p.ID),
	})
	writeDispatchResult(w, h.Logger, "nvmeof.port.create", out, err)
}

func (h *NvmeofWriteHandler) DeletePort(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id < 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_port_id", "port id is invalid")
		return
	}
	out, derr := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNvmeofPortDelete,
		Target:    strconv.Itoa(id),
		Payload:   jobs.NvmeofPortDeletePayload{ID: id},
		Command:   "nvmet port delete " + strconv.Itoa(id),
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "nvmeof:port:" + strconv.Itoa(id),
	})
	writeDispatchResult(w, h.Logger, "nvmeof.port.delete", out, derr)
}

// ---------- port <-> subsystem link ----------

type nvmeofLinkReq struct {
	NQN string `json:"nqn"`
}

func (h *NvmeofWriteHandler) LinkSubsystem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id < 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_port_id", "port id is invalid")
		return
	}
	var body nvmeofLinkReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if !validateNQNHandler(body.NQN) {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nqn", "nqn is invalid")
		return
	}
	out, derr := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNvmeofPortLink,
		Target:    strconv.Itoa(id),
		Payload:   jobs.NvmeofPortLinkPayload{PortID: id, NQN: body.NQN},
		Command:   "nvmet port link " + strconv.Itoa(id) + " " + body.NQN,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "nvmeof.port.link", out, derr)
}

func (h *NvmeofWriteHandler) UnlinkSubsystem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id < 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_port_id", "port id is invalid")
		return
	}
	nqn, ok := decodeNQN(chi.URLParam(r, "nqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_nqn", "nqn is invalid")
		return
	}
	out, derr := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindNvmeofPortUnlink,
		Target:    strconv.Itoa(id),
		Payload:   jobs.NvmeofPortUnlinkPayload{PortID: id, NQN: nqn},
		Command:   "nvmet port unlink " + strconv.Itoa(id) + " " + nqn,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "nvmeof.port.unlink", out, derr)
}
