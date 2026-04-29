// Package handlers — Marketplaces registry HTTP layer.
//
// The marketplace registry decouples the plugin engine from a single
// hardcoded source. Operators can add additional marketplaces
// (TrueCharts, third-party publishers, internal mirrors); each carries
// its own pinned cosign trust key. The locked novanas-official entry
// is seeded at boot and cannot be deleted or disabled.
package handlers

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/plugins"
)

// MarketplacesHandler exposes /api/v1/marketplaces/*. When Store is
// nil every route responds 503.
type MarketplacesHandler struct {
	Logger *slog.Logger
	Store  plugins.MarketplacesStore
	Multi  *plugins.MultiMarketplaceClient
	// HTTP, when non-nil, is used for index-URL HEAD probes and
	// trust-key URL fetches. Defaults to a 15s-timeout client.
	HTTP *http.Client
	// AddedBy returns the Keycloak `sub` for the calling identity. The
	// production wiring derives it from the auth middleware; tests may
	// supply a fake. nil falls back to "" for added_by.
	AddedBy func(*http.Request) string
}

func (h *MarketplacesHandler) ready(w http.ResponseWriter) bool {
	if h == nil || h.Store == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_available", "marketplaces subsystem not configured")
		return false
	}
	return true
}

func (h *MarketplacesHandler) httpClient() *http.Client {
	if h.HTTP != nil {
		return h.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// MarketplaceResponse is the JSON shape returned by the
// list/get/create/update endpoints. The trust-key PEM is intentionally
// included — operators need to be able to audit the pinned key.
type MarketplaceResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	IndexURL    string `json:"indexUrl"`
	TrustKeyURL string `json:"trustKeyUrl"`
	TrustKeyPEM string `json:"trustKeyPem"`
	Locked      bool   `json:"locked"`
	Enabled     bool   `json:"enabled"`
	AddedBy     string `json:"addedBy,omitempty"`
	AddedAt     string `json:"addedAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

func toMarketplaceResponse(m plugins.Marketplace) MarketplaceResponse {
	r := MarketplaceResponse{
		ID:          m.ID.String(),
		Name:        m.Name,
		IndexURL:    m.IndexURL,
		TrustKeyURL: m.TrustKeyURL,
		TrustKeyPEM: m.TrustKeyPEM,
		Locked:      m.Locked,
		Enabled:     m.Enabled,
		AddedBy:     m.AddedBy,
	}
	if !m.AddedAt.IsZero() {
		r.AddedAt = m.AddedAt.UTC().Format(time.RFC3339)
	}
	if !m.UpdatedAt.IsZero() {
		r.UpdatedAt = m.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return r
}

// MarketplaceCreateRequest is the body of POST /marketplaces.
type MarketplaceCreateRequest struct {
	Name        string `json:"name"`
	IndexURL    string `json:"indexUrl"`
	TrustKeyURL string `json:"trustKeyUrl"`
}

// MarketplacePatchRequest is the body of PATCH /marketplaces/{id}.
// Only fields that are non-nil are applied.
type MarketplacePatchRequest struct {
	Enabled *bool `json:"enabled,omitempty"`
	// RefreshTrustKey, when true, re-fetches the trust_key_url and
	// pins the returned PEM. This is the same operation as
	// POST /marketplaces/{id}/refresh-trust-key, exposed via PATCH for
	// convenience.
	RefreshTrustKey bool `json:"refreshTrustKey,omitempty"`
}

// List handles GET /marketplaces.
func (h *MarketplacesHandler) List(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	rows, err := h.Store.List(r.Context())
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	out := make([]MarketplaceResponse, 0, len(rows))
	for _, m := range rows {
		out = append(out, toMarketplaceResponse(m))
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, out)
}

// Get handles GET /marketplaces/{id}.
func (h *MarketplacesHandler) Get(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	m, err := h.Store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, plugins.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "marketplace not found")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, toMarketplaceResponse(m))
}

// Create handles POST /marketplaces. Validates the index URL is
// reachable (HTTP HEAD) and the trust key URL returns a parseable
// PEM-encoded public key BEFORE persisting.
func (h *MarketplacesHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var req MarketplaceCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.IndexURL = strings.TrimSpace(req.IndexURL)
	req.TrustKeyURL = strings.TrimSpace(req.TrustKeyURL)
	if req.Name == "" || req.IndexURL == "" || req.TrustKeyURL == "" {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_argument",
			"name, indexUrl, and trustKeyUrl are required")
		return
	}
	if req.Name == plugins.OfficialMarketplaceName {
		middleware.WriteError(w, http.StatusConflict, "reserved_name",
			"the novanas-official name is reserved for the locked entry")
		return
	}
	// Reachability check on the index URL (HEAD; some servers reject
	// HEAD with 405, which we accept as "reachable").
	if err := h.probeURL(r.Context(), req.IndexURL); err != nil {
		middleware.WriteError(w, http.StatusBadGateway, "index_unreachable", err.Error())
		return
	}
	// Fetch + parse the trust key.
	pemStr, err := h.fetchTrustKey(r.Context(), req.TrustKeyURL)
	if err != nil {
		middleware.WriteError(w, http.StatusBadGateway, "trust_key_unreachable", err.Error())
		return
	}
	if err := validatePublicKeyPEM(pemStr); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_trust_key", err.Error())
		return
	}
	addedBy := ""
	if h.AddedBy != nil {
		addedBy = h.AddedBy(r)
	}
	m := plugins.Marketplace{
		ID:          uuid.New(),
		Name:        req.Name,
		IndexURL:    req.IndexURL,
		TrustKeyURL: req.TrustKeyURL,
		TrustKeyPEM: pemStr,
		Locked:      false,
		Enabled:     true,
		AddedBy:     addedBy,
	}
	created, err := h.Store.Create(r.Context(), m)
	if err != nil {
		// UNIQUE constraint on name → 409.
		if strings.Contains(err.Error(), "already exists") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			middleware.WriteError(w, http.StatusConflict, "already_exists", err.Error())
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusCreated, toMarketplaceResponse(created))
}

// Patch handles PATCH /marketplaces/{id}. Supports flipping `enabled`
// and (optionally) refreshing the pinned trust key from
// `trust_key_url`.
func (h *MarketplacesHandler) Patch(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	var req MarketplacePatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	current, err := h.Store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, plugins.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "marketplace not found")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	updated := current
	if req.Enabled != nil {
		if current.Locked && !*req.Enabled {
			middleware.WriteError(w, http.StatusConflict, "locked_marketplace",
				"the locked novanas-official entry cannot be disabled")
			return
		}
		row, err := h.Store.UpdateEnabled(r.Context(), id, *req.Enabled)
		if err != nil {
			middleware.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		updated = row
	}
	if req.RefreshTrustKey {
		row, err := h.refreshTrustKey(r.Context(), updated)
		if err != nil {
			middleware.WriteError(w, http.StatusBadGateway, "trust_key_unreachable", err.Error())
			return
		}
		updated = row
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, toMarketplaceResponse(updated))
}

// Delete handles DELETE /marketplaces/{id}.
func (h *MarketplacesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	current, err := h.Store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, plugins.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "marketplace not found")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if current.Locked {
		middleware.WriteError(w, http.StatusConflict, "locked_marketplace",
			"the locked novanas-official entry cannot be deleted")
		return
	}
	if err := h.Store.Delete(r.Context(), id); err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RefreshTrustKey handles POST /marketplaces/{id}/refresh-trust-key.
// Re-fetches the .pub from trust_key_url and pins it.
func (h *MarketplacesHandler) RefreshTrustKey(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	current, err := h.Store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, plugins.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "marketplace not found")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	updated, err := h.refreshTrustKey(r.Context(), current)
	if err != nil {
		middleware.WriteError(w, http.StatusBadGateway, "trust_key_unreachable", err.Error())
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, toMarketplaceResponse(updated))
}

func (h *MarketplacesHandler) refreshTrustKey(ctx context.Context, current plugins.Marketplace) (plugins.Marketplace, error) {
	if current.TrustKeyURL == "" {
		return plugins.Marketplace{}, fmt.Errorf("no trust_key_url pinned (locked entry?)")
	}
	pemStr, err := h.fetchTrustKey(ctx, current.TrustKeyURL)
	if err != nil {
		return plugins.Marketplace{}, err
	}
	if err := validatePublicKeyPEM(pemStr); err != nil {
		return plugins.Marketplace{}, fmt.Errorf("fetched key invalid: %w", err)
	}
	return h.Store.UpdateTrustKey(ctx, current.ID, pemStr)
}

func (h *MarketplacesHandler) probeURL(ctx context.Context, url string) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return fmt.Errorf("invalid url: %q", url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return err
	}
	resp, err := h.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 405 (Method Not Allowed) is OK — many static hosts reject HEAD.
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return nil
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		return nil
	}
	return fmt.Errorf("HEAD %s: HTTP %d", url, resp.StatusCode)
}

func (h *MarketplacesHandler) fetchTrustKey(ctx context.Context, url string) (string, error) {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return "", fmt.Errorf("invalid url: %q", url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := h.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10)) // 64 KiB cap
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// validatePublicKeyPEM parses pem and confirms the embedded key is one
// of the cosign-supported types (ECDSA, RSA, Ed25519). Used to refuse
// junk before we persist a row.
func validatePublicKeyPEM(s string) error {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return errors.New("not PEM-encoded")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse PKIX public key: %w", err)
	}
	if pub == nil {
		return errors.New("nil public key")
	}
	return nil
}
