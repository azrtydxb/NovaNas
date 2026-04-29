// Package handlers — Alertmanager pass-through.
//
// nova-api authenticates the caller against its own RBAC and forwards
// the request to a loopback-bound Alertmanager instance using its own
// (no-auth) admin URL. The caller's bearer token is intentionally NOT
// forwarded — those are different auth domains.
package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
)

// AlertsHandler exposes /api/v1/alerts* and /api/v1/alert-silences*.
//
// UpstreamURL is the Alertmanager base URL (e.g. http://127.0.0.1:9093).
// HTTP is optional; when nil http.DefaultClient is used.
type AlertsHandler struct {
	Logger      *slog.Logger
	UpstreamURL string
	HTTP        *http.Client
}

func (h *AlertsHandler) httpc() *http.Client {
	if h.HTTP != nil {
		return h.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// upstream builds the AM /api/v2 URL for the relative path p (e.g.
// "/alerts").
func (h *AlertsHandler) upstream(p string, q url.Values) string {
	base := strings.TrimRight(h.UpstreamURL, "/")
	u := base + "/api/v2" + p
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// proxyJSON performs a JSON request/response pass-through. The upstream
// status is preserved (with a few translations for known AM errors) and
// the body is streamed back verbatim.
func (h *AlertsHandler) proxyJSON(ctx context.Context, w http.ResponseWriter, method, target string, body io.Reader) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "alertmanager_request", err.Error())
		return
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.httpc().Do(req)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Warn("alertmanager unreachable", "err", err, "target", target)
		}
		middleware.WriteError(w, http.StatusBadGateway, "alertmanager_unreachable", "Alertmanager upstream is not reachable")
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// ListAlerts handles GET /api/v1/alerts.
func (h *AlertsHandler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(r.Context(), w, http.MethodGet, h.upstream("/alerts", r.URL.Query()), nil)
}

// GetAlert handles GET /api/v1/alerts/{fingerprint}. Alertmanager has no
// per-alert detail endpoint; we filter the /alerts list by fingerprint
// server-side.
func (h *AlertsHandler) GetAlert(w http.ResponseWriter, r *http.Request) {
	fp := strings.TrimSpace(chi.URLParam(r, "fingerprint"))
	if fp == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_fingerprint", "fingerprint is required")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, h.upstream("/alerts", nil), nil)
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "alertmanager_request", err.Error())
		return
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.httpc().Do(req)
	if err != nil {
		middleware.WriteError(w, http.StatusBadGateway, "alertmanager_unreachable", "Alertmanager upstream is not reachable")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}
	var alerts []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		middleware.WriteError(w, http.StatusBadGateway, "alertmanager_decode", err.Error())
		return
	}
	for _, a := range alerts {
		if v, _ := a["fingerprint"].(string); v == fp {
			middleware.WriteJSON(w, h.Logger, http.StatusOK, a)
			return
		}
	}
	middleware.WriteError(w, http.StatusNotFound, "not_found", "no alert with that fingerprint")
}

// ListSilences handles GET /api/v1/alert-silences.
func (h *AlertsHandler) ListSilences(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(r.Context(), w, http.MethodGet, h.upstream("/silences", r.URL.Query()), nil)
}

// CreateSilence handles POST /api/v1/alert-silences. Body is forwarded
// verbatim (Alertmanager's own Silence schema).
func (h *AlertsHandler) CreateSilence(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_body", err.Error())
		return
	}
	if !json.Valid(body) {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
		return
	}
	h.proxyJSON(r.Context(), w, http.MethodPost, h.upstream("/silences", nil), strings.NewReader(string(body)))
}

// DeleteSilence handles DELETE /api/v1/alert-silences/{id}.
func (h *AlertsHandler) DeleteSilence(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", "id is required")
		return
	}
	h.proxyJSON(r.Context(), w, http.MethodDelete, h.upstream("/silence/"+url.PathEscape(id), nil), nil)
}

// ListReceivers handles GET /api/v1/alert-receivers. AM exposes this at
// /api/v2/receivers — read-only.
func (h *AlertsHandler) ListReceivers(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(r.Context(), w, http.MethodGet, h.upstream("/receivers", nil), nil)
}
