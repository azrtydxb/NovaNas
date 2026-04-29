package novanas

import (
	"context"
	"errors"
	"net/http"
)

// SMTPConfig mirrors the SMTPConfigDTO in the server's notifications
// handler. Password is write-only — GET responses always return "***"
// for the password field; PUT requests that send "***" leave the
// stored password unchanged.
type SMTPConfig struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	FromAddress  string `json:"fromAddress"`
	TLSMode      string `json:"tlsMode"`
	MaxPerMinute int    `json:"maxPerMinute,omitempty"`
}

// SMTPTestResult is the response of POST /notifications/smtp/test.
type SMTPTestResult struct {
	Status string `json:"status"`
	To     string `json:"to"`
}

// GetSMTPConfig returns the currently stored SMTP relay config. The
// password is always returned as "***".
func (c *Client) GetSMTPConfig(ctx context.Context) (*SMTPConfig, error) {
	var out SMTPConfig
	if _, err := c.do(ctx, http.MethodGet, "/notifications/smtp", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PutSMTPConfig replaces the SMTP relay config. To preserve the
// existing password without re-typing it, set Password to "***".
func (c *Client) PutSMTPConfig(ctx context.Context, cfg SMTPConfig) (*SMTPConfig, error) {
	if cfg.Host == "" || cfg.Port == 0 || cfg.FromAddress == "" {
		return nil, errors.New("novanas: SMTPConfig.Host, Port, FromAddress are required")
	}
	var out SMTPConfig
	if _, err := c.do(ctx, http.MethodPut, "/notifications/smtp", nil, cfg, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SendSMTPTest sends a synchronous test email. The relay's accept-or-
// reject verdict is surfaced in err: a 200 response means the relay
// accepted the message; a 502 (returned as *APIError with code
// "smtp_error") means the relay refused or the connection failed.
func (c *Client) SendSMTPTest(ctx context.Context, to string) (*SMTPTestResult, error) {
	if to == "" {
		return nil, errors.New("novanas: to is required")
	}
	body := map[string]string{"to": to}
	var out SMTPTestResult
	if _, err := c.do(ctx, http.MethodPost, "/notifications/smtp/test", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
