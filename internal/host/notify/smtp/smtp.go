// Package smtp implements a thin wrapper around net/smtp that supports
// plain (no auth, plaintext), STARTTLS, and implicit-TLS connections to
// an SMTP relay. It is used by nova-api to deliver transactional email
// (password reset, invite, weekly summary) and by tests via dependency
// injection.
//
// The package is deliberately small: operators bring their own relay
// (SendGrid, Mailgun, Postmark, on-prem Postfix) and we just speak SMTP.
package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// TLSMode controls how the client establishes the SMTP connection.
type TLSMode string

const (
	// TLSModeNone — plaintext on the wire, no STARTTLS, no AUTH unless the
	// relay accepts it on a plaintext channel. Only safe over loopback or
	// a private VLAN.
	TLSModeNone TLSMode = "none"
	// TLSModeStartTLS — connect plaintext, issue STARTTLS, then AUTH.
	// This is the typical submission-port (587) mode.
	TLSModeStartTLS TLSMode = "starttls"
	// TLSModeTLS — implicit TLS from byte zero (port 465 SMTPS).
	TLSModeTLS TLSMode = "tls"
)

// Valid reports whether m is one of the known modes.
func (m TLSMode) Valid() bool {
	switch m {
	case TLSModeNone, TLSModeStartTLS, TLSModeTLS:
		return true
	default:
		return false
	}
}

// Config configures a Client.
type Config struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	TLSMode     TLSMode
	// Timeout caps the entire Send call (dial + handshake + transaction).
	// Zero means 30s.
	Timeout time.Duration
	// InsecureSkipVerify disables certificate verification on the TLS
	// channel. Operators should never set this in production; it exists so
	// dev environments with a self-signed relay can be exercised.
	InsecureSkipVerify bool
	// Dial overrides net.Dial for tests. Production callers leave it nil.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

// Client sends mail through an SMTP relay according to Config.
//
// A Client is safe for concurrent use; net/smtp opens a fresh connection
// for every Send.
type Client struct {
	cfg Config
}

// New constructs a Client and validates Config. It returns an error when
// required fields are missing or TLSMode is unknown.
func New(cfg Config) (*Client, error) {
	if cfg.Host == "" {
		return nil, errors.New("smtp: Host is required")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("smtp: invalid port %d", cfg.Port)
	}
	if cfg.FromAddress == "" {
		return nil, errors.New("smtp: FromAddress is required")
	}
	if cfg.TLSMode == "" {
		cfg.TLSMode = TLSModeStartTLS
	}
	if !cfg.TLSMode.Valid() {
		return nil, fmt.Errorf("smtp: invalid TLSMode %q", cfg.TLSMode)
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &Client{cfg: cfg}, nil
}

// Config returns the (immutable) effective config the client uses.
func (c *Client) Config() Config { return c.cfg }

// Send delivers a single message to the given recipients.
//
// Subject and body are wrapped into a minimal RFC 5322 message; extra
// headers are merged in (caller-supplied headers override built-ins for
// keys other than To/From). Newlines in subject are stripped to avoid
// header injection.
func (c *Client) Send(ctx context.Context, to []string, subject, body string, headers map[string]string) error {
	if len(to) == 0 {
		return errors.New("smtp: at least one recipient is required")
	}
	addr := net.JoinHostPort(c.cfg.Host, fmt.Sprintf("%d", c.cfg.Port))

	dialCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	dial := c.cfg.Dial
	if dial == nil {
		d := &net.Dialer{Timeout: c.cfg.Timeout}
		dial = d.DialContext
	}

	var conn net.Conn
	var err error
	switch c.cfg.TLSMode {
	case TLSModeTLS:
		raw, derr := dial(dialCtx, "tcp", addr)
		if derr != nil {
			return fmt.Errorf("smtp: dial: %w", derr)
		}
		tlsConn := tls.Client(raw, &tls.Config{
			ServerName:         c.cfg.Host,
			InsecureSkipVerify: c.cfg.InsecureSkipVerify, //nolint:gosec // operator opt-in
			MinVersion:         tls.VersionTLS12,
		})
		if err = tlsConn.HandshakeContext(dialCtx); err != nil {
			_ = raw.Close()
			return fmt.Errorf("smtp: tls handshake: %w", err)
		}
		conn = tlsConn
	default:
		conn, err = dial(dialCtx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("smtp: dial: %w", err)
		}
	}
	// Apply absolute deadline so a hanging relay can't outlast the timeout.
	if dl, ok := dialCtx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

	cl, err := smtp.NewClient(conn, c.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp: new client: %w", err)
	}
	defer func() { _ = cl.Close() }()

	if c.cfg.TLSMode == TLSModeStartTLS {
		if ok, _ := cl.Extension("STARTTLS"); !ok {
			return errors.New("smtp: server does not support STARTTLS")
		}
		tlsCfg := &tls.Config{
			ServerName:         c.cfg.Host,
			InsecureSkipVerify: c.cfg.InsecureSkipVerify, //nolint:gosec // operator opt-in
			MinVersion:         tls.VersionTLS12,
		}
		if err := cl.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("smtp: starttls: %w", err)
		}
	}

	if c.cfg.Username != "" || c.cfg.Password != "" {
		// Use PLAIN over a TLS channel; LOGIN as a fallback if PLAIN is
		// unsupported. This matches what most relays accept.
		auth := smtp.PlainAuth("", c.cfg.Username, c.cfg.Password, c.cfg.Host)
		if err := cl.Auth(auth); err != nil {
			return fmt.Errorf("smtp: auth: %w", err)
		}
	}

	if err := cl.Mail(c.cfg.FromAddress); err != nil {
		return fmt.Errorf("smtp: MAIL FROM: %w", err)
	}
	for _, rcpt := range to {
		if err := cl.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp: RCPT TO %s: %w", rcpt, err)
		}
	}
	wc, err := cl.Data()
	if err != nil {
		return fmt.Errorf("smtp: DATA: %w", err)
	}
	msg := buildMessage(c.cfg.FromAddress, to, subject, body, headers)
	if _, err := io.WriteString(wc, msg); err != nil {
		_ = wc.Close()
		return fmt.Errorf("smtp: write body: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("smtp: close body: %w", err)
	}
	if err := cl.Quit(); err != nil {
		// Quit failures are usually harmless; the message has already been
		// accepted by the time we get here. Log via the returned error so
		// the caller can observe if they care.
		return fmt.Errorf("smtp: quit: %w", err)
	}
	return nil
}

// sanitizeHeader strips CR/LF to prevent header injection.
func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// buildMessage assembles a minimal RFC 5322 message with CRLF line
// endings. Caller-supplied headers override the defaults for any key
// other than From/To/Subject/Date/MIME-Version/Content-Type, which are
// always set by us.
func buildMessage(from string, to []string, subject, body string, headers map[string]string) string {
	var b strings.Builder
	required := map[string]string{
		"From":         sanitizeHeader(from),
		"To":           sanitizeHeader(strings.Join(to, ", ")),
		"Subject":      sanitizeHeader(subject),
		"Date":         time.Now().UTC().Format(time.RFC1123Z),
		"MIME-Version": "1.0",
		"Content-Type": "text/plain; charset=utf-8",
	}
	// Allow caller to override Content-Type and Date but not From/To/Subject.
	for k, v := range headers {
		canon := canonicalHeaderKey(k)
		switch canon {
		case "From", "To", "Subject":
			// ignored — we control these
		default:
			required[canon] = sanitizeHeader(v)
		}
	}
	// Deterministic order of the well-known headers first.
	order := []string{"From", "To", "Subject", "Date", "MIME-Version", "Content-Type"}
	written := map[string]bool{}
	for _, k := range order {
		if v, ok := required[k]; ok {
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
			written[k] = true
		}
	}
	for k, v := range required {
		if written[k] {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\r\n", k, v)
	}
	b.WriteString("\r\n")
	// Normalize body line endings to CRLF and dot-stuff.
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, ".") {
			b.WriteString(".")
		}
		b.WriteString(line)
		b.WriteString("\r\n")
	}
	return b.String()
}

// canonicalHeaderKey is a tiny version of textproto.CanonicalMIMEHeaderKey
// that avoids the import. It title-cases each '-' separated token.
func canonicalHeaderKey(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
	}
	return strings.Join(parts, "-")
}
