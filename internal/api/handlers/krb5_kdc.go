// Package handlers — embedded MIT KDC management endpoints.
//
// These endpoints expose principal CRUD plus KDC status for the
// machine-credential identity model (see internal/host/krb5/kdc.go).
// Per-user principals are NOT supported; Keycloak remains the source
// of truth for human identities.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/krb5"
)

// Krb5KDC is the contract used by Krb5KDCHandler. Concrete impl is
// *krb5.KDCManager.
type Krb5KDC interface {
	Status(ctx context.Context) (*krb5.KDCStatus, error)
	ListPrincipals(ctx context.Context) ([]string, error)
	GetPrincipal(ctx context.Context, name string) (*krb5.PrincipalInfo, error)
	CreatePrincipal(ctx context.Context, spec krb5.CreatePrincipalSpec) (*krb5.PrincipalInfo, error)
	DeletePrincipal(ctx context.Context, name string) error
	GenerateKeytab(ctx context.Context, name, dir string) ([]byte, error)
}

// Krb5KDCHandler exposes the KDC status + principal CRUD endpoints.
type Krb5KDCHandler struct {
	Logger *slog.Logger
	KDC    Krb5KDC
}

// principalNameFromPath URL-decodes the path param. Operators that
// pass a literal '@' or '/' must URL-encode it; chi delivers the
// already-decoded segment, but we run it through PathUnescape defensively
// for the case where the operator double-encoded.
func principalNameFromPath(r *http.Request) string {
	raw := chi.URLParam(r, "name")
	if dec, err := url.PathUnescape(raw); err == nil {
		return dec
	}
	return raw
}

// GetStatus handles GET /api/v1/krb5/kdc/status.
func (h *Krb5KDCHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	st, err := h.KDC.Status(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("krb5 kdc status", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to read kdc status")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, st)
}

// ListPrincipals handles GET /api/v1/krb5/principals.
func (h *Krb5KDCHandler) ListPrincipals(w http.ResponseWriter, r *http.Request) {
	names, err := h.KDC.ListPrincipals(r.Context())
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("krb5 list principals", "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to list principals")
		return
	}
	if names == nil {
		names = []string{}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, names)
}

// GetPrincipal handles GET /api/v1/krb5/principals/{name}.
func (h *Krb5KDCHandler) GetPrincipal(w http.ResponseWriter, r *http.Request) {
	name := principalNameFromPath(r)
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "principal name required")
		return
	}
	info, err := h.KDC.GetPrincipal(r.Context(), name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "principal not found")
			return
		}
		if h.Logger != nil {
			h.Logger.Error("krb5 get principal", "err", err, "name", name)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to get principal")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, info)
}

// principalCreateReq is the wire body for POST /api/v1/krb5/principals.
type principalCreateReq struct {
	Name     string `json:"name"`
	Randkey  bool   `json:"randkey,omitempty"`
	Password string `json:"password,omitempty"`
}

// CreatePrincipal handles POST /api/v1/krb5/principals.
func (h *Krb5KDCHandler) CreatePrincipal(w http.ResponseWriter, r *http.Request) {
	var req principalCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if req.Name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "name required")
		return
	}
	if req.Randkey && req.Password != "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_args", "randkey and password are mutually exclusive")
		return
	}
	info, err := h.KDC.CreatePrincipal(r.Context(), krb5.CreatePrincipalSpec{
		Name:     req.Name,
		Randkey:  req.Randkey,
		Password: req.Password,
	})
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("krb5 create principal", "err", err, "name", req.Name)
		}
		middleware.WriteError(w, http.StatusBadRequest, "kdc_error", err.Error())
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusCreated, info)
}

// DeletePrincipal handles DELETE /api/v1/krb5/principals/{name}.
func (h *Krb5KDCHandler) DeletePrincipal(w http.ResponseWriter, r *http.Request) {
	name := principalNameFromPath(r)
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "principal name required")
		return
	}
	if err := h.KDC.DeletePrincipal(r.Context(), name); err != nil {
		if h.Logger != nil {
			h.Logger.Error("krb5 delete principal", "err", err, "name", name)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to delete principal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetPrincipalKeytab handles POST /api/v1/krb5/principals/{name}/keytab.
// Returns the raw MIT keytab bytes (application/octet-stream).
//
// The KVNO is rotated by ktadd; existing distributed keytabs become
// invalid. Caller is responsible for atomic re-distribution.
func (h *Krb5KDCHandler) GetPrincipalKeytab(w http.ResponseWriter, r *http.Request) {
	name := principalNameFromPath(r)
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "principal name required")
		return
	}
	data, err := h.KDC.GenerateKeytab(r.Context(), name, "")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "principal not found")
			return
		}
		if h.Logger != nil {
			h.Logger.Error("krb5 generate keytab", "err", err, "name", name)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", "failed to generate keytab")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="principal.keytab"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
