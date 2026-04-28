// Package handlers — iSCSI write (dispatch) endpoints.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/iscsi"
	"github.com/novanas/nova-nas/internal/jobs"
)

// IscsiWriteHandler handles mutating iSCSI operations by dispatching jobs.
type IscsiWriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
}

// ---------- validators (mirror the host package, kept here so we can
// reject early at the API boundary with a 400 instead of a job failure) ----------

func validateIQNHandler(iqn string) bool {
	if !strings.HasPrefix(iqn, "iqn.") || strings.HasPrefix(iqn, "-") {
		return false
	}
	for _, r := range iqn {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-' || r == ':'
		if !ok {
			return false
		}
	}
	return len(iqn) > 4
}

func decodeIQN(raw string) (string, bool) {
	s, err := url.PathUnescape(raw)
	if err != nil {
		return "", false
	}
	return s, validateIQNHandler(s)
}

func validateTransportTCP(t string) bool {
	switch t {
	case "", "tcp", "iser":
		return true
	}
	return false
}

// ---------- target ----------

type iscsiTargetCreateReq struct {
	IQN string `json:"iqn"`
}

func (h *IscsiWriteHandler) CreateTarget(w http.ResponseWriter, r *http.Request) {
	var req iscsiTargetCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if !validateIQNHandler(req.IQN) {
		middleware.WriteError(w, http.StatusBadRequest, "bad_iqn", "iqn is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindIscsiTargetCreate,
		Target:    req.IQN,
		Payload:   jobs.IscsiTargetCreatePayload{IQN: req.IQN},
		Command:   "targetcli /iscsi create " + req.IQN,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "iscsi:target:" + req.IQN,
	})
	writeDispatchResult(w, h.Logger, "iscsi.target.create", out, err)
}

func (h *IscsiWriteHandler) DestroyTarget(w http.ResponseWriter, r *http.Request) {
	iqn, ok := decodeIQN(chi.URLParam(r, "iqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_iqn", "iqn is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindIscsiTargetDestroy,
		Target:    iqn,
		Payload:   jobs.IscsiTargetDestroyPayload{IQN: iqn},
		Command:   "targetcli /iscsi delete " + iqn,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "iscsi:target:" + iqn,
	})
	writeDispatchResult(w, h.Logger, "iscsi.target.destroy", out, err)
}

// ---------- portal ----------

func (h *IscsiWriteHandler) CreatePortal(w http.ResponseWriter, r *http.Request) {
	iqn, ok := decodeIQN(chi.URLParam(r, "iqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_iqn", "iqn is invalid")
		return
	}
	var p iscsi.Portal
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
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
	if !validateTransportTCP(p.Transport) {
		middleware.WriteError(w, http.StatusBadRequest, "bad_transport", "transport must be tcp or iser")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindIscsiPortalCreate,
		Target:    iqn,
		Payload:   jobs.IscsiPortalCreatePayload{IQN: iqn, Portal: p},
		Command:   "targetcli portal create " + iqn,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "iscsi.portal.create", out, err)
}

func (h *IscsiWriteHandler) DeletePortal(w http.ResponseWriter, r *http.Request) {
	iqn, ok := decodeIQN(chi.URLParam(r, "iqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_iqn", "iqn is invalid")
		return
	}
	ip := chi.URLParam(r, "ip")
	if net.ParseIP(ip) == nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_ip", "ip is invalid")
		return
	}
	port, err := strconv.Atoi(chi.URLParam(r, "port"))
	if err != nil || port < 1 || port > 65535 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_port", "port is invalid")
		return
	}
	p := iscsi.Portal{IP: ip, Port: port}
	out, derr := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindIscsiPortalDelete,
		Target:    iqn,
		Payload:   jobs.IscsiPortalDeletePayload{IQN: iqn, Portal: p},
		Command:   "targetcli portal delete " + iqn,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "iscsi.portal.delete", out, derr)
}

// ---------- LUN ----------

func (h *IscsiWriteHandler) CreateLUN(w http.ResponseWriter, r *http.Request) {
	iqn, ok := decodeIQN(chi.URLParam(r, "iqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_iqn", "iqn is invalid")
		return
	}
	var lun iscsi.LUN
	if err := json.NewDecoder(r.Body).Decode(&lun); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if lun.ID < 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_lun_id", "lun id must be >= 0")
		return
	}
	if strings.TrimSpace(lun.Backstore) == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_backstore", "backstore required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindIscsiLUNCreate,
		Target:    iqn,
		Payload:   jobs.IscsiLUNCreatePayload{IQN: iqn, LUN: lun},
		Command:   "targetcli lun create " + iqn,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "iscsi.lun.create", out, err)
}

func (h *IscsiWriteHandler) DeleteLUN(w http.ResponseWriter, r *http.Request) {
	iqn, ok := decodeIQN(chi.URLParam(r, "iqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_iqn", "iqn is invalid")
		return
	}
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id < 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_lun_id", "lun id is invalid")
		return
	}
	out, derr := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindIscsiLUNDelete,
		Target:    iqn,
		Payload:   jobs.IscsiLUNDeletePayload{IQN: iqn, ID: id},
		Command:   "targetcli lun delete " + iqn,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "iscsi.lun.delete", out, derr)
}

// ---------- ACL ----------

func (h *IscsiWriteHandler) CreateACL(w http.ResponseWriter, r *http.Request) {
	iqn, ok := decodeIQN(chi.URLParam(r, "iqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_iqn", "iqn is invalid")
		return
	}
	var acl iscsi.ACL
	if err := json.NewDecoder(r.Body).Decode(&acl); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if !validateIQNHandler(acl.InitiatorIQN) {
		middleware.WriteError(w, http.StatusBadRequest, "bad_initiator_iqn", "initiator iqn is invalid")
		return
	}
	// CHAP secret length: per RFC 3720 / iscsi pkg constraint: 12-16 chars when present.
	if acl.CHAPSecret != "" {
		if n := len(acl.CHAPSecret); n < 12 || n > 16 {
			middleware.WriteError(w, http.StatusBadRequest, "bad_chap_secret", "CHAP secret must be 12-16 characters")
			return
		}
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindIscsiACLCreate,
		Target:    iqn,
		Payload:   jobs.IscsiACLCreatePayload{IQN: iqn, ACL: acl},
		Command:   "targetcli acl create " + iqn,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "iscsi.acl.create", out, err)
}

func (h *IscsiWriteHandler) DeleteACL(w http.ResponseWriter, r *http.Request) {
	iqn, ok := decodeIQN(chi.URLParam(r, "iqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_iqn", "iqn is invalid")
		return
	}
	initiator, ok := decodeIQN(chi.URLParam(r, "initiatorIqn"))
	if !ok {
		middleware.WriteError(w, http.StatusBadRequest, "bad_initiator_iqn", "initiator iqn is invalid")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindIscsiACLDelete,
		Target:    iqn,
		Payload:   jobs.IscsiACLDeletePayload{IQN: iqn, InitiatorIQN: initiator},
		Command:   "targetcli acl delete " + iqn,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	writeDispatchResult(w, h.Logger, "iscsi.acl.delete", out, err)
}

// ---------- saveconfig ----------

func (h *IscsiWriteHandler) SaveConfig(w http.ResponseWriter, r *http.Request) {
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindIscsiSaveConfig,
		Target:    "iscsi",
		Payload:   jobs.IscsiSaveConfigPayload{},
		Command:   "targetcli saveconfig",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "iscsi:saveconfig",
	})
	writeDispatchResult(w, h.Logger, "iscsi.saveconfig", out, err)
}
