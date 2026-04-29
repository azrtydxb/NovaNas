// Package handlers — Loki pass-through.
//
// nova-api validates the caller's RBAC then forwards LogQL queries to a
// loopback-bound Loki upstream. The bearer token is not propagated:
// upstream is unauthenticated and trusts the loopback boundary.
package handlers

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
)

// LogsHandler exposes /api/v1/logs/* endpoints. UpstreamURL is the Loki
// base URL (e.g. http://127.0.0.1:3100). HTTP is optional.
type LogsHandler struct {
	Logger      *slog.Logger
	UpstreamURL string
	HTTP        *http.Client
}

func (h *LogsHandler) httpc() *http.Client {
	if h.HTTP != nil {
		return h.HTTP
	}
	// Range queries with very long ranges may take a while to materialize;
	// no per-request timeout — caller cancellation flows through ctx.
	return http.DefaultClient
}

func (h *LogsHandler) upstream(p string, q url.Values) string {
	base := strings.TrimRight(h.UpstreamURL, "/")
	u := base + "/loki/api/v1" + p
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// proxyStream performs a streaming pass-through. The upstream body is
// copied chunk-by-chunk so very large LogQL ranges don't buffer in
// memory; ctx cancellation propagates upstream via the request context.
func (h *LogsHandler) proxyStream(ctx context.Context, w http.ResponseWriter, target string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "loki_request", err.Error())
		return
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.httpc().Do(req)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Warn("loki unreachable", "err", err, "target", target)
		}
		middleware.WriteError(w, http.StatusBadGateway, "loki_unreachable", "Loki upstream is not reachable")
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	// Use chunked transfer for ranges that may be large.
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(resp.StatusCode)
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if rerr == io.EOF {
			return
		}
		if rerr != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

// QueryRange handles GET /api/v1/logs/query — Loki LogQL range query.
func (h *LogsHandler) QueryRange(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.URL.Query().Get("query")) == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_query", "query is required")
		return
	}
	h.proxyStream(r.Context(), w, h.upstream("/query_range", r.URL.Query()))
}

// QueryInstant handles GET /api/v1/logs/query/instant — Loki LogQL
// instant query.
func (h *LogsHandler) QueryInstant(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.URL.Query().Get("query")) == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_query", "query is required")
		return
	}
	h.proxyStream(r.Context(), w, h.upstream("/query", r.URL.Query()))
}

// Labels handles GET /api/v1/logs/labels.
func (h *LogsHandler) Labels(w http.ResponseWriter, r *http.Request) {
	h.proxyStream(r.Context(), w, h.upstream("/labels", r.URL.Query()))
}

// LabelValues handles GET /api/v1/logs/labels/{name}/values.
func (h *LogsHandler) LabelValues(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(chi.URLParam(r, "name"))
	if name == "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_label", "label name is required")
		return
	}
	h.proxyStream(r.Context(), w, h.upstream("/label/"+url.PathEscape(name)+"/values", r.URL.Query()))
}

// Series handles GET /api/v1/logs/series.
func (h *LogsHandler) Series(w http.ResponseWriter, r *http.Request) {
	h.proxyStream(r.Context(), w, h.upstream("/series", r.URL.Query()))
}

// Tail handles GET /api/v1/logs/tail. The upstream Loki tail endpoint is
// a WebSocket; nova-api does not currently terminate WebSocket clients
// itself — callers should target the upstream directly via a
// network-level reverse proxy. This stub returns 501 so the surface is
// documented and discoverable but unimplemented in v1.
func (h *LogsHandler) Tail(w http.ResponseWriter, r *http.Request) {
	middleware.WriteError(w, http.StatusNotImplemented, "tail_not_implemented",
		"WebSocket pass-through to Loki /tail is not yet implemented; see docs/logs/README.md")
}
