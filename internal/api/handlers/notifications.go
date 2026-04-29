// Package handlers — outbound notifications (SMTP relay) endpoints.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"

	"github.com/novanas/nova-nas/internal/api/middleware"
	smtpmgr "github.com/novanas/nova-nas/internal/host/notify/smtp"
)

// NotificationsHandler exposes /api/v1/notifications/smtp endpoints.
//
// It is wired in cmd/nova-api/main.go. When Mgr is nil the handler is
// not registered.
type NotificationsHandler struct {
	Logger *slog.Logger
	Mgr    *smtpmgr.Manager
}

// SMTPConfigDTO is the JSON body returned by GET and accepted by PUT.
// The Password field is write-only — GET responses always echo "***".
type SMTPConfigDTO struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	FromAddress  string `json:"fromAddress"`
	TLSMode      string `json:"tlsMode"`
	MaxPerMinute int    `json:"maxPerMinute,omitempty"`
}

// passwordRedaction is the placeholder echoed in GET responses. PUT
// requests that send this exact value are interpreted as "leave the
// stored password alone" — operators can update Host/From without
// re-typing the password.
const passwordRedaction = "***"

// GetConfig handles GET /api/v1/notifications/smtp.
func (h *NotificationsHandler) GetConfig(w http.ResponseWriter, _ *http.Request) {
	cfg := h.Mgr.Config()
	dto := SMTPConfigDTO{
		Host:         cfg.Host,
		Port:         cfg.Port,
		Username:     cfg.Username,
		FromAddress:  cfg.FromAddress,
		TLSMode:      string(cfg.TLSMode),
		MaxPerMinute: h.Mgr.MaxPerMinute(),
	}
	if cfg.Password != "" {
		dto.Password = passwordRedaction
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, dto)
}

// PutConfig handles PUT /api/v1/notifications/smtp.
func (h *NotificationsHandler) PutConfig(w http.ResponseWriter, r *http.Request) {
	var dto SMTPConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if dto.Host == "" || dto.Port == 0 || dto.FromAddress == "" {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "host, port, and fromAddress are required")
		return
	}
	if _, err := mail.ParseAddress(dto.FromAddress); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "fromAddress is not a valid RFC 5322 address")
		return
	}
	mode := smtpmgr.TLSMode(strings.ToLower(strings.TrimSpace(dto.TLSMode)))
	if mode == "" {
		mode = smtpmgr.TLSModeStartTLS
	}
	if !mode.Valid() {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "tlsMode must be one of none|starttls|tls")
		return
	}

	// Preserve the stored password if the client echoed the redaction
	// placeholder (typical "edit only the host" flow).
	current := h.Mgr.Config()
	password := dto.Password
	if password == passwordRedaction {
		password = current.Password
	}

	cfg := smtpmgr.Config{
		Host:        dto.Host,
		Port:        dto.Port,
		Username:    dto.Username,
		Password:    password,
		FromAddress: dto.FromAddress,
		TLSMode:     mode,
	}
	if err := h.Mgr.SetConfig(cfg); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if dto.MaxPerMinute > 0 {
		h.Mgr.SetMaxPerMinute(dto.MaxPerMinute)
	}
	// Echo back the redacted form so callers immediately see the post-update state.
	out := SMTPConfigDTO{
		Host:         cfg.Host,
		Port:         cfg.Port,
		Username:     cfg.Username,
		FromAddress:  cfg.FromAddress,
		TLSMode:      string(cfg.TLSMode),
		MaxPerMinute: h.Mgr.MaxPerMinute(),
	}
	if cfg.Password != "" {
		out.Password = passwordRedaction
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, out)
}

// TestRequest is the request body for POST /notifications/smtp/test.
type TestRequest struct {
	To string `json:"to"`
}

// PostTest handles POST /api/v1/notifications/smtp/test. Synchronous —
// the relay error (if any) is surfaced to the caller so the operator
// gets an immediate verdict.
func (h *NotificationsHandler) PostTest(w http.ResponseWriter, r *http.Request) {
	var req TestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
		return
	}
	if req.To == "" {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "to is required")
		return
	}
	if _, err := mail.ParseAddress(req.To); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "to is not a valid RFC 5322 address")
		return
	}
	ctx, cancel := contextWithDefault(r.Context())
	defer cancel()
	if err := h.Mgr.SendTest(ctx, req.To); err != nil {
		if errors.Is(err, smtpmgr.ErrNotConfigured) {
			middleware.WriteError(w, http.StatusBadRequest, "not_configured", "SMTP relay is not configured")
			return
		}
		if h.Logger != nil {
			h.Logger.Warn("smtp test", "err", err)
		}
		middleware.WriteError(w, http.StatusBadGateway, "smtp_error", err.Error())
		return
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, map[string]string{"status": "sent", "to": req.To})
}

// contextWithDefault returns a derived context using the request context
// so middleware-set deadlines propagate, while letting the SMTP timeout
// (configured on the Manager) drive the actual cap.
func contextWithDefault(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}
