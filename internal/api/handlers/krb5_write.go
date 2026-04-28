// Package handlers — Kerberos write (dispatch) endpoints.
package handlers

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/krb5"
	"github.com/novanas/nova-nas/internal/jobs"
)

// Krb5WriteHandler handles mutating Kerberos operations by dispatching jobs.
type Krb5WriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
}

// SetConfig handles PUT /api/v1/krb5/config. The body is a Krb5Config.
func (h *Krb5WriteHandler) SetConfig(w http.ResponseWriter, r *http.Request) {
	var cfg krb5.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if cfg.DefaultRealm == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_realm", "defaultRealm required")
		return
	}
	if len(cfg.Realms) == 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_realms", "at least one realm required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindKrb5SetConfig,
		Target:    "krb5",
		Payload:   jobs.Krb5SetConfigPayload{Config: cfg},
		Command:   "krb5 set-config",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "krb5:config",
	})
	writeDispatchResult(w, h.Logger, "krb5.config.set", out, err)
}

// SetIdmapd handles PUT /api/v1/krb5/idmapd.
func (h *Krb5WriteHandler) SetIdmapd(w http.ResponseWriter, r *http.Request) {
	var cfg krb5.IdmapdConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if cfg.Domain == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_domain", "domain required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindKrb5SetIdmapd,
		Target:    "krb5.idmapd",
		Payload:   jobs.Krb5SetIdmapdPayload{Config: cfg},
		Command:   "krb5 set-idmapd",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "krb5:idmapd",
	})
	writeDispatchResult(w, h.Logger, "krb5.idmapd.set", out, err)
}

// keytabUploadReq is the wire shape for PUT /api/v1/krb5/keytab. We accept
// base64 explicitly so the handler can return a clean 400 on decode error;
// json.RawMessage of []byte would also work but produces a less specific
// error envelope.
type keytabUploadReq struct {
	Data string `json:"data"`
}

// UploadKeytab handles PUT /api/v1/krb5/keytab. Body shape:
//
//	{"data": "<base64-encoded keytab bytes>"}
//
// We decode base64 at the API boundary so a malformed encoding is surfaced
// as 400 rather than as an opaque async job failure. The decoded raw bytes
// are then passed to the worker via Krb5UploadKeytabPayload, where the
// []byte field is re-encoded as base64 by encoding/json — round-tripping is
// safe because both ends are encoding/json.
func (h *Krb5WriteHandler) UploadKeytab(w http.ResponseWriter, r *http.Request) {
	var req keytabUploadReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if req.Data == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_data", "data required")
		return
	}
	raw, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_base64", "data is not valid base64")
		return
	}
	if len(raw) == 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_data", "decoded keytab is empty")
		return
	}
	out, derr := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindKrb5UploadKeytab,
		Target:    "krb5.keytab",
		Payload:   jobs.Krb5UploadKeytabPayload{Data: raw},
		Command:   "krb5 upload-keytab",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "krb5:keytab",
	})
	writeDispatchResult(w, h.Logger, "krb5.keytab.upload", out, derr)
}

// DeleteKeytab handles DELETE /api/v1/krb5/keytab.
func (h *Krb5WriteHandler) DeleteKeytab(w http.ResponseWriter, r *http.Request) {
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindKrb5DeleteKeytab,
		Target:    "krb5.keytab",
		Payload:   jobs.Krb5DeleteKeytabPayload{},
		Command:   "krb5 delete-keytab",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "krb5:keytab",
	})
	writeDispatchResult(w, h.Logger, "krb5.keytab.delete", out, err)
}
