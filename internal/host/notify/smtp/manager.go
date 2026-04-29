package smtp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Manager wraps a Client with operator-tunable runtime state: the
// effective config (rotatable at runtime via the API), a per-recipient
// leaky-bucket rate limiter, and a tiny in-memory outbox of failed
// messages awaiting retry.
//
// The outbox is intentionally in-memory for now. A future migration
// will move it behind sqlc; until then operators who care about
// at-most-once-loss after a process restart should rely on the relay's
// own queueing (every modern SMTP relay buffers).
type Manager struct {
	mu     sync.RWMutex
	cfg    Config
	client *Client

	// rate-limit
	rlMu      sync.Mutex
	bucket    map[string]*tokenBucket
	maxPerMin int

	// outbox
	obMu   sync.Mutex
	outbox []OutboxEntry
	nextID int64
}

// OutboxStatus is the lifecycle state of an outbox entry.
type OutboxStatus string

const (
	OutboxPending OutboxStatus = "pending"
	OutboxSent    OutboxStatus = "sent"
	OutboxFailed  OutboxStatus = "failed"
)

// OutboxEntry is one queued message. Stored in memory only.
type OutboxEntry struct {
	ID         int64
	To         []string
	Subject    string
	Body       string
	Headers    map[string]string
	Status     OutboxStatus
	LastError  string
	Attempts   int
	EnqueuedAt time.Time
	UpdatedAt  time.Time
}

// NewManager constructs a Manager. maxPerMinute <= 0 means use the
// default (30). Pass an empty Config to defer wiring until SetConfig is
// called.
func NewManager(cfg Config, maxPerMinute int) (*Manager, error) {
	m := &Manager{
		bucket:    map[string]*tokenBucket{},
		maxPerMin: maxPerMinute,
	}
	if maxPerMinute <= 0 {
		m.maxPerMin = 30
	}
	if cfg.Host != "" {
		if err := m.SetConfig(cfg); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// Config returns the current config (unredacted; callers must redact at
// the API boundary).
func (m *Manager) Config() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// SetConfig replaces the active config. The new client is constructed
// eagerly so configuration errors are returned to the operator.
func (m *Manager) SetConfig(cfg Config) error {
	cl, err := New(cfg)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
	m.client = cl
	return nil
}

// MaxPerMinute returns the current per-recipient rate limit.
func (m *Manager) MaxPerMinute() int {
	m.rlMu.Lock()
	defer m.rlMu.Unlock()
	return m.maxPerMin
}

// SetMaxPerMinute updates the per-recipient leaky-bucket capacity.
func (m *Manager) SetMaxPerMinute(n int) {
	if n <= 0 {
		n = 30
	}
	m.rlMu.Lock()
	defer m.rlMu.Unlock()
	m.maxPerMin = n
	// drop existing buckets so the new rate takes effect immediately
	m.bucket = map[string]*tokenBucket{}
}

// Send sends synchronously. On rate-limit miss, returns ErrRateLimited
// without enqueueing. On transport error, the message is appended to
// the outbox with status=failed and the error is returned (the operator
// can retry via RetryOutbox).
func (m *Manager) Send(ctx context.Context, to []string, subject, body string, headers map[string]string) error {
	m.mu.RLock()
	cl := m.client
	m.mu.RUnlock()
	if cl == nil {
		return ErrNotConfigured
	}
	if !m.allow(to) {
		return ErrRateLimited
	}
	if err := cl.Send(ctx, to, subject, body, headers); err != nil {
		m.appendOutbox(to, subject, body, headers, OutboxFailed, err.Error())
		return err
	}
	return nil
}

// Enqueue appends a message to the outbox without trying to send it.
// Useful for the worker model: an HTTP request enqueues, a background
// loop drains.
func (m *Manager) Enqueue(to []string, subject, body string, headers map[string]string) OutboxEntry {
	return m.appendOutbox(to, subject, body, headers, OutboxPending, "")
}

// Outbox returns a copy of the current outbox. Useful for observability.
func (m *Manager) Outbox() []OutboxEntry {
	m.obMu.Lock()
	defer m.obMu.Unlock()
	out := make([]OutboxEntry, len(m.outbox))
	copy(out, m.outbox)
	return out
}

// DrainPending attempts to send all pending entries in FIFO order. On
// success an entry transitions to sent; on failure to failed with
// LastError populated. Returns the number of entries successfully sent.
func (m *Manager) DrainPending(ctx context.Context) (int, error) {
	m.mu.RLock()
	cl := m.client
	m.mu.RUnlock()
	if cl == nil {
		return 0, ErrNotConfigured
	}
	m.obMu.Lock()
	pending := make([]int, 0)
	for i, e := range m.outbox {
		if e.Status == OutboxPending {
			pending = append(pending, i)
		}
	}
	m.obMu.Unlock()

	sent := 0
	for _, idx := range pending {
		m.obMu.Lock()
		e := m.outbox[idx]
		m.obMu.Unlock()
		err := cl.Send(ctx, e.To, e.Subject, e.Body, e.Headers)
		m.obMu.Lock()
		m.outbox[idx].Attempts++
		m.outbox[idx].UpdatedAt = time.Now().UTC()
		if err != nil {
			m.outbox[idx].Status = OutboxFailed
			m.outbox[idx].LastError = err.Error()
		} else {
			m.outbox[idx].Status = OutboxSent
			m.outbox[idx].LastError = ""
			sent++
		}
		m.obMu.Unlock()
	}
	return sent, nil
}

// SendTest issues a single message synchronously, bypassing both the
// rate limit and the outbox so the operator gets an immediate verdict.
func (m *Manager) SendTest(ctx context.Context, to string) error {
	m.mu.RLock()
	cl := m.client
	m.mu.RUnlock()
	if cl == nil {
		return ErrNotConfigured
	}
	body := fmt.Sprintf("This is a test email from NovaNAS.\n\nSent at %s.\n", time.Now().UTC().Format(time.RFC3339))
	return cl.Send(ctx, []string{to}, "NovaNAS SMTP test", body, map[string]string{"X-NovaNAS-Test": "1"})
}

func (m *Manager) appendOutbox(to []string, subject, body string, headers map[string]string, status OutboxStatus, lastErr string) OutboxEntry {
	m.obMu.Lock()
	defer m.obMu.Unlock()
	m.nextID++
	now := time.Now().UTC()
	e := OutboxEntry{
		ID:         m.nextID,
		To:         append([]string{}, to...),
		Subject:    subject,
		Body:       body,
		Headers:    cloneHeaders(headers),
		Status:     status,
		LastError:  lastErr,
		EnqueuedAt: now,
		UpdatedAt:  now,
	}
	m.outbox = append(m.outbox, e)
	// keep outbox bounded
	if len(m.outbox) > 1000 {
		m.outbox = m.outbox[len(m.outbox)-1000:]
	}
	return e
}

func cloneHeaders(h map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = v
	}
	return out
}

// ----- rate limiter -----

type tokenBucket struct {
	tokens float64
	last   time.Time
}

func (m *Manager) allow(to []string) bool {
	m.rlMu.Lock()
	defer m.rlMu.Unlock()
	now := time.Now()
	rate := float64(m.maxPerMin) / 60.0 // tokens per second
	for _, addr := range to {
		b, ok := m.bucket[addr]
		if !ok {
			b = &tokenBucket{tokens: float64(m.maxPerMin), last: now}
			m.bucket[addr] = b
		}
		dt := now.Sub(b.last).Seconds()
		b.last = now
		b.tokens += dt * rate
		if b.tokens > float64(m.maxPerMin) {
			b.tokens = float64(m.maxPerMin)
		}
		if b.tokens < 1 {
			return false
		}
	}
	// Second pass: consume a token from each.
	for _, addr := range to {
		m.bucket[addr].tokens -= 1
	}
	return true
}

// ErrRateLimited signals the per-recipient leaky bucket is empty.
var ErrRateLimited = errors.New("smtp: rate limited")

// ErrNotConfigured signals the manager has no SMTP client yet.
var ErrNotConfigured = errors.New("smtp: not configured")
