// Package handlers — Notification Center event endpoints (the bell).
//
// This file is deliberately separate from notifications.go (which is
// SMTP-relay configuration). They share the /notifications path
// prefix but are different concerns; see internal/notifycenter for
// the event store, and internal/host/notify/smtp for the relay.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/auth"
	"github.com/novanas/nova-nas/internal/notifycenter"
)

// NotificationsEventsHandler exposes /api/v1/notifications/events*.
type NotificationsEventsHandler struct {
	Logger *slog.Logger
	Mgr    *notifycenter.Manager
}

// nceHeartbeatInterval matches jobs_sse: 15s keeps idle SSE connections
// alive through proxies (nginx 60s default, AWS ALB 60s).
const nceHeartbeatInterval = 15 * time.Second

// List handles GET /notifications/events.
func (h *NotificationsEventsHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.Mgr == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_ready", "notification center not initialized")
		return
	}
	subj := userSubject(r)
	q := r.URL.Query()
	f := notifycenter.ListFilter{
		Severity:         notifycenter.Severity(q.Get("severity")),
		Source:           notifycenter.Source(q.Get("source")),
		OnlyUnread:       q.Get("unread") == "true",
		IncludeDismissed: q.Get("includeDismissed") == "true",
		IncludeSnoozed:   q.Get("includeSnoozed") == "true" || q.Get("onlySnoozed") == "true",
		OnlySnoozed:      q.Get("onlySnoozed") == "true",
	}
	if lim := q.Get("limit"); lim != "" {
		if n, err := strconv.Atoi(lim); err == nil && n > 0 {
			f.Limit = int32(n)
		}
	}
	if cur := q.Get("cursor"); cur != "" {
		// Cursor is "<rfc3339>|<uuid>" — opaque to clients, just fed
		// back unchanged on the next page.
		if i := strings.Index(cur, "|"); i > 0 {
			if t, err := time.Parse(time.RFC3339Nano, cur[:i]); err == nil {
				f.CursorTime = t
				f.CursorID = cur[i+1:]
			}
		}
	}
	if f.Severity != "" && !f.Severity.Valid() {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_query", "severity must be info|warning|critical")
		return
	}
	if f.Source != "" && !f.Source.Valid() {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_query", "source must be alertmanager|jobs|audit|system")
		return
	}
	events, err := h.Mgr.ListForUser(r.Context(), subj, f)
	if err != nil {
		h.logErr("list", err)
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", "failed to load notifications")
		return
	}
	resp := struct {
		Items      []notifycenter.Event `json:"items"`
		NextCursor string               `json:"nextCursor,omitempty"`
	}{Items: events}
	if len(events) > 0 && f.Limit > 0 && int32(len(events)) == f.Limit {
		last := events[len(events)-1]
		resp.NextCursor = last.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, resp)
}

// UnreadCount handles GET /notifications/events/unread-count.
func (h *NotificationsEventsHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	if h.Mgr == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_ready", "notification center not initialized")
		return
	}
	subj := userSubject(r)
	n, err := h.Mgr.UnreadCountForUser(r.Context(), subj)
	if err != nil {
		h.logErr("unread", err)
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", "failed to count notifications")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, map[string]int64{"count": n})
}

// MarkRead handles POST /notifications/events/{id}/read.
func (h *NotificationsEventsHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	if h.Mgr == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_ready", "notification center not initialized")
		return
	}
	subj := userSubject(r)
	id := chi.URLParam(r, "id")
	if err := h.Mgr.MarkRead(r.Context(), subj, id); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, map[string]string{"status": "read"})
}

// MarkDismissed handles POST /notifications/events/{id}/dismiss.
func (h *NotificationsEventsHandler) MarkDismissed(w http.ResponseWriter, r *http.Request) {
	if h.Mgr == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_ready", "notification center not initialized")
		return
	}
	subj := userSubject(r)
	id := chi.URLParam(r, "id")
	if err := h.Mgr.MarkDismissed(r.Context(), subj, id); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, map[string]string{"status": "dismissed"})
}

// SnoozeRequest is the body of POST /notifications/events/{id}/snooze.
type SnoozeRequest struct {
	Until time.Time `json:"until"`
}

// Snooze handles POST /notifications/events/{id}/snooze.
func (h *NotificationsEventsHandler) Snooze(w http.ResponseWriter, r *http.Request) {
	if h.Mgr == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_ready", "notification center not initialized")
		return
	}
	subj := userSubject(r)
	id := chi.URLParam(r, "id")
	var req SnoozeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if req.Until.IsZero() {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "until is required")
		return
	}
	if err := h.Mgr.Snooze(r.Context(), subj, id, req.Until); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, map[string]any{
		"status": "snoozed",
		"until":  req.Until.UTC().Format(time.RFC3339Nano),
	})
}

// MarkAllRead handles POST /notifications/events/read-all.
func (h *NotificationsEventsHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	if h.Mgr == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_ready", "notification center not initialized")
		return
	}
	subj := userSubject(r)
	if err := h.Mgr.MarkAllRead(r.Context(), subj); err != nil {
		h.logErr("mark all", err)
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", "failed to mark all read")
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, map[string]string{"status": "ok"})
}

// Stream handles GET /notifications/events/stream — the SSE channel.
//
// Frame format (one message per `data:` line, terminated by a blank
// line per the W3C SSE spec):
//
//	event: notification
//	data: {"id":"...", ...}
//
// Keepalive frames are sent as SSE comments (`: keepalive\n\n`) every
// nceHeartbeatInterval to keep idle connections alive across proxies.
func (h *NotificationsEventsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	if h.Mgr == nil {
		middleware.WriteError(w, http.StatusServiceUnavailable, "not_ready", "notification center not initialized")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		middleware.WriteError(w, http.StatusInternalServerError, "no_flusher", "stream unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	ch, unsubscribe := h.Mgr.Subscribe()
	defer unsubscribe()

	// Initial heartbeat so clients can detect connect-success without
	// waiting for the first event or the 15s keepalive tick.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	keepalive := time.NewTicker(nceHeartbeatInterval)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			buf, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: notification\ndata: %s\n\n", buf)
			flusher.Flush()
		}
	}
}

// userSubject extracts the calling identity's subject. When auth is
// disabled (dev/test) the subject is the empty string; per-user state
// then collapses to a single "anonymous" bucket which is fine because
// no real users exist in that mode.
func userSubject(r *http.Request) string {
	if id, ok := auth.IdentityFromContext(r.Context()); ok && id != nil {
		return id.Subject
	}
	return ""
}

func (h *NotificationsEventsHandler) logErr(op string, err error) {
	if h.Logger == nil {
		return
	}
	h.Logger.Warn("notifications.events", "op", op, "err", err)
}
