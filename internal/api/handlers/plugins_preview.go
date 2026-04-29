package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/auth"
	"github.com/novanas/nova-nas/internal/plugins"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// PluginsPreviewAuditor is the minimum surface the preview handler
// needs to write a structured "plugins.preview" audit row. It mirrors
// EncryptionAuditor; the same store wiring satisfies both.
type PluginsPreviewAuditor interface {
	InsertAudit(ctx context.Context, arg storedb.InsertAuditParams) error
}

// PluginsPreviewHandler serves GET /plugins/index/{name}/manifest.
// It is intentionally separate from PluginsHandler so the preview
// path's narrow dependencies (marketplace + verifier only — no
// Manager, no Router) are explicit at wiring time.
type PluginsPreviewHandler struct {
	Logger      *slog.Logger
	Marketplace *plugins.MarketplaceClient
	Verifier    *plugins.Verifier
	// Auditor is optional; nil disables the structured "plugins.preview"
	// audit row (the global Audit middleware still records the HTTP
	// request itself).
	Auditor PluginsPreviewAuditor
}

// Preview is the HTTP handler. Wire it under
//
//	GET /api/v1/plugins/index/{name}/manifest?version=1.0.0
//
// inside the PermPluginsRead group.
func (h *PluginsPreviewHandler) Preview(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Marketplace == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_available", "marketplace not configured")
		return
	}
	name := chi.URLParam(r, "name")
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "missing_name", "plugin name is required")
		return
	}
	version := r.URL.Query().Get("version")
	if version == "" {
		// Required so the consent dialog and the eventual install
		// pin to the same released artifact. Aurora always knows the
		// version it picked from the index.
		middleware.WriteError(w, http.StatusBadRequest, "missing_version", "version query parameter is required")
		return
	}

	res, err := plugins.PreviewPlugin(r.Context(), h.Marketplace, h.Verifier, name, version)
	if err != nil {
		var pe *plugins.PreviewError
		if errors.As(err, &pe) {
			switch pe.Code {
			case plugins.PreviewErrNotFound:
				middleware.WriteError(w, http.StatusNotFound, "not_found", pe.Error())
				return
			case plugins.PreviewErrMarketplaceUnreach:
				middleware.WriteError(w, http.StatusBadGateway, "marketplace_unreachable", pe.Error())
				return
			case plugins.PreviewErrSignatureInvalid:
				// 422 (Unprocessable Entity) — the upstream artifact
				// exists but cannot be trusted. Distinct from a generic
				// 502 so Aurora can surface a tampering warning.
				middleware.WriteError(w, http.StatusUnprocessableEntity, "signature_invalid", pe.Error())
				return
			case plugins.PreviewErrManifestInvalid:
				middleware.WriteError(w, http.StatusUnprocessableEntity, "manifest_invalid", pe.Error())
				return
			}
		}
		if h.Logger != nil {
			h.Logger.Error("plugins.preview", "name", name, "version", version, "err", err)
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Structured audit row. The action mirrors the encryption.recover
	// pattern so audit consumers see "plugins.preview" verbatim. We
	// capture the verified tarball SHA so a subsequent install can be
	// cross-checked against the consent record.
	if h.Auditor != nil {
		id, _ := auth.IdentityFromContext(r.Context())
		actor := actorFromIdentity(id)
		payload, _ := json.Marshal(map[string]string{
			"name":          name,
			"version":       version,
			"tarballSha256": res.TarballSHA256,
			"caller":        actor,
			"at":            time.Now().UTC().Format(time.RFC3339),
		})
		if auditErr := h.Auditor.InsertAudit(r.Context(), storedb.InsertAuditParams{
			Actor:     actorPtr(actor),
			Action:    "plugins.preview",
			Target:    name,
			RequestID: middleware.RequestIDOf(r.Context()),
			Payload:   payload,
			Result:    "accepted",
		}); auditErr != nil && h.Logger != nil {
			h.Logger.Warn("plugins.preview audit insert failed", "err", auditErr, "name", name)
		}
	}

	middleware.WriteJSON(w, h.Logger, http.StatusOK, res)
}
