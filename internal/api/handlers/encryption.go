package handlers

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/auth"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// EncryptionAuditor is the minimum the recovery handler needs to write
// an explicit audit row. It deliberately mirrors middleware.AuditQuerier
// so the same store wiring satisfies both. Wrapping the call here (in
// addition to the global Audit middleware) means recovery actions are
// recorded with a structured "encryption.recover" action even if the
// middleware ever changes its action format.
type EncryptionAuditor interface {
	InsertAudit(ctx context.Context, arg storedb.InsertAuditParams) error
}

// EncryptionHandler exposes the per-dataset native-encryption API.
//
// Routes:
//
//	POST /api/v1/datasets/{fullname}/encryption          -> initialize
//	POST /api/v1/datasets/{fullname}/encryption/load-key
//	POST /api/v1/datasets/{fullname}/encryption/unload-key
//	POST /api/v1/datasets/{fullname}/encryption/recover  -> admin-only
//
// The Mgr does the real work; this handler is a thin marshaling layer
// plus the recovery audit hook.
type EncryptionHandler struct {
	Logger  *slog.Logger
	Mgr     *dataset.EncryptionManager
	Auditor EncryptionAuditor // optional; nil disables explicit audit row
}

// EncryptionInitRequest is the body for POST .../encryption.
type EncryptionInitRequest struct {
	// Type is "filesystem" or "volume". Required.
	Type string `json:"type"`
	// VolumeSizeBytes is required for type=volume.
	VolumeSizeBytes uint64 `json:"volumeSizeBytes,omitempty"`
	// Algorithm overrides the default aes-256-gcm.
	Algorithm string `json:"algorithm,omitempty"`
	// Properties are extra `-o k=v` pairs passed to zfs create. The
	// keys "encryption", "keyformat", and "keylocation" are reserved
	// and rejected.
	Properties map[string]string `json:"properties,omitempty"`
}

// EncryptionInitResponse is what the initialize endpoint returns. It
// deliberately does NOT include the raw key — the caller can fetch it
// via the recover endpoint if they need an offline copy.
type EncryptionInitResponse struct {
	Dataset   string `json:"dataset"`
	Algorithm string `json:"algorithm"`
	Created   string `json:"created"`
}

// EncryptionRecoverResponse contains the unwrapped raw key as a hex
// string. Hex is chosen over base64 because operators hand-paste it
// into `zfs load-key -L file://` workflows where the canonical raw-
// key file format is binary, not base64.
type EncryptionRecoverResponse struct {
	Dataset string `json:"dataset"`
	// KeyHex is the 64-character hex encoding of the 32-byte raw key.
	KeyHex string `json:"keyHex"`
}

// Initialize provisions a new encrypted dataset.
func (h *EncryptionHandler) Initialize(w http.ResponseWriter, r *http.Request) {
	full, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	var body EncryptionInitRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	if body.Type != "filesystem" && body.Type != "volume" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_type", "type must be filesystem or volume")
		return
	}
	if body.Type == "volume" && body.VolumeSizeBytes == 0 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_size", "volumeSizeBytes required for volume")
		return
	}
	parent, name, ok2 := splitParentName(full)
	if !ok2 {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "dataset has no parent")
		return
	}
	spec := &dataset.CreateSpec{
		Parent:               parent,
		Name:                 name,
		Type:                 body.Type,
		VolumeSizeBytes:      body.VolumeSizeBytes,
		Properties:           body.Properties,
		EncryptionEnabled:    true,
		EncryptionAlgorithm:  body.Algorithm,
	}
	if h.Mgr == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "no_tpm", "encryption manager not configured (no TPM?)")
		return
	}
	if _, err := h.Mgr.Initialize(r.Context(), full, spec); err != nil {
		h.Logger.Error("encryption.initialize", "dataset", full, "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "init_failed", err.Error())
		return
	}
	alg := body.Algorithm
	if alg == "" {
		alg = dataset.DefaultEncryptionAlgorithm
	}
	resp := EncryptionInitResponse{
		Dataset:   full,
		Algorithm: alg,
		Created:   time.Now().UTC().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// LoadKey reads the wrapped blob from the secrets backend, TPM-
// unwraps, and feeds the raw key to ZFS over stdin.
func (h *EncryptionHandler) LoadKey(w http.ResponseWriter, r *http.Request) {
	full, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	if h.Mgr == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "no_tpm", "encryption manager not configured")
		return
	}
	if err := h.Mgr.LoadKey(r.Context(), full); err != nil {
		h.Logger.Error("encryption.load_key", "dataset", full, "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "load_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UnloadKey detaches the in-memory key from ZFS. The wrapped blob in
// the secrets backend is untouched.
func (h *EncryptionHandler) UnloadKey(w http.ResponseWriter, r *http.Request) {
	full, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	if h.Mgr == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "no_tpm", "encryption manager not configured")
		return
	}
	if err := h.Mgr.UnloadKey(r.Context(), full); err != nil {
		h.Logger.Error("encryption.unload_key", "dataset", full, "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "unload_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Recover returns the unwrapped raw 32-byte ZFS key as hex. This is
// the break-glass capability: it lets an operator move an encrypted
// dataset to a host with a different TPM (or print the key for offline
// safekeeping). Every call is audit-logged with caller identity,
// target dataset, and timestamp; the response body is NOT logged
// (since it's the actual secret).
func (h *EncryptionHandler) Recover(w http.ResponseWriter, r *http.Request) {
	full, ok := decodeDatasetFullname(w, r)
	if !ok {
		return
	}
	if h.Mgr == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "no_tpm", "encryption manager not configured")
		return
	}

	// Identify the caller for the audit row. When auth is disabled
	// (test mode), Identity is nil — we still log the action with an
	// empty actor, which is semantically equivalent to "system".
	id, _ := auth.IdentityFromContext(r.Context())

	rawKey, err := h.Mgr.Recover(r.Context(), full)
	result := "accepted"
	if err != nil {
		result = "rejected"
	}

	// Best-effort: even if the recovery failed, record the attempt.
	if h.Auditor != nil {
		actor := actorFromIdentity(id)
		payload, _ := json.Marshal(map[string]string{
			"dataset":  full,
			"caller":   actor,
			"at":       time.Now().UTC().Format(time.RFC3339),
		})
		if auditErr := h.Auditor.InsertAudit(r.Context(), storedb.InsertAuditParams{
			Actor:     actorPtr(actor),
			Action:    "encryption.recover",
			Target:    full,
			RequestID: middleware.RequestIDOf(r.Context()),
			Payload:   payload,
			Result:    result,
		}); auditErr != nil && h.Logger != nil {
			h.Logger.Error("encryption.recover audit insert failed",
				"err", auditErr, "dataset", full)
		}
	}

	if err != nil {
		h.Logger.Error("encryption.recover", "dataset", full, "err", err)
		if errors.Is(err, errNotFound{}) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "no escrowed key for dataset")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "recover_failed", err.Error())
		return
	}
	defer zeroBytes(rawKey)

	resp := EncryptionRecoverResponse{
		Dataset: full,
		KeyHex:  hex.EncodeToString(rawKey),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// errNotFound is a sentinel for parity with secrets.ErrNotFound; we
// don't import secrets here to keep handler dependencies narrow.
type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func actorFromIdentity(id *auth.Identity) string {
	if id == nil {
		return ""
	}
	if id.Subject != "" {
		return id.Subject
	}
	if id.PreferredName != "" {
		return id.PreferredName
	}
	return id.Email
}

func actorPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// splitParentName splits "tank/a/b" into ("tank/a", "b"). Returns
// false when there is no '/' separator (i.e. a bare pool, which is
// not a valid encrypted-dataset target — the operator would encrypt
// the pool's root via zpool create -O instead).
func splitParentName(full string) (string, string, bool) {
	for i := len(full) - 1; i >= 0; i-- {
		if full[i] == '/' {
			return full[:i], full[i+1:], true
		}
	}
	return "", "", false
}
