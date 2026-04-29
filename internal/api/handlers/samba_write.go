// Package handlers — Samba write (dispatch) endpoints.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/samba"
	"github.com/novanas/nova-nas/internal/jobs"
)

// SambaWriteHandler dispatches mutating Samba operations as jobs.
type SambaWriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
}

// CreateShare handles POST /api/v1/samba/shares.
func (h *SambaWriteHandler) CreateShare(w http.ResponseWriter, r *http.Request) {
	var s samba.Share
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if s.Name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "share name required")
		return
	}
	if s.Path == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_path", "share path required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSambaShareCreate,
		Target:    s.Name,
		Payload:   jobs.SambaShareCreatePayload{Share: s},
		Command:   "smb share create " + s.Name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "samba:share:" + s.Name,
	})
	writeDispatchResult(w, h.Logger, "samba.share.create", out, err)
}

// UpdateShare handles PATCH /api/v1/samba/shares/{name}. Body is the
// full Share; URL name takes precedence on mismatch (we error rather
// than silently rename).
func (h *SambaWriteHandler) UpdateShare(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	var s samba.Share
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if s.Name == "" {
		s.Name = name
	} else if s.Name != name {
		middleware.WriteError(w, http.StatusBadRequest, "name_mismatch", "URL name and body name disagree")
		return
	}
	if s.Path == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_path", "share path required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSambaShareUpdate,
		Target:    name,
		Payload:   jobs.SambaShareUpdatePayload{Share: s},
		Command:   "smb share update " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "samba:share:" + name,
	})
	writeDispatchResult(w, h.Logger, "samba.share.update", out, err)
}

// DeleteShare handles DELETE /api/v1/samba/shares/{name}.
func (h *SambaWriteHandler) DeleteShare(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSambaShareDelete,
		Target:    name,
		Payload:   jobs.SambaShareDeletePayload{Name: name},
		Command:   "smb share delete " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "samba:share:" + name,
	})
	writeDispatchResult(w, h.Logger, "samba.share.delete", out, err)
}

// Reload handles POST /api/v1/samba/reload.
func (h *SambaWriteHandler) Reload(w http.ResponseWriter, r *http.Request) {
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSambaReload,
		Target:    "samba",
		Payload:   jobs.SambaReloadPayload{},
		Command:   "smbcontrol smbd reload-config",
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "samba:reload",
	})
	writeDispatchResult(w, h.Logger, "samba.reload", out, err)
}

// sambaUserCredentials is the body for AddUser and SetUserPassword.
type sambaUserCredentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password"`
}

// AddUser handles POST /api/v1/samba/users.
func (h *SambaWriteHandler) AddUser(w http.ResponseWriter, r *http.Request) {
	var body sambaUserCredentials
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if body.Username == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_username", "username required")
		return
	}
	if body.Password == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_password", "password required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSambaUserAdd,
		Target:    body.Username,
		Payload:   jobs.SambaUserAddPayload{Username: body.Username, Password: body.Password},
		Command:   "smbpasswd -a " + body.Username,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "samba:user:" + body.Username,
	})
	writeDispatchResult(w, h.Logger, "samba.user.add", out, err)
}

// DeleteUser handles DELETE /api/v1/samba/users/{username}.
func (h *SambaWriteHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if username == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_username", "username required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSambaUserDelete,
		Target:    username,
		Payload:   jobs.SambaUserDeletePayload{Username: username},
		Command:   "smbpasswd -x " + username,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "samba:user:" + username,
	})
	writeDispatchResult(w, h.Logger, "samba.user.delete", out, err)
}

// SetUserPassword handles PUT /api/v1/samba/users/{username}/password.
func (h *SambaWriteHandler) SetUserPassword(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if username == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_username", "username required")
		return
	}
	var body sambaUserCredentials
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if body.Password == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_password", "password required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindSambaUserSetPassword,
		Target:    username,
		Payload:   jobs.SambaUserSetPasswordPayload{Username: username, Password: body.Password},
		Command:   "smbpasswd " + username,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "samba:user:" + username,
	})
	writeDispatchResult(w, h.Logger, "samba.user.set_password", out, err)
}
