// Package notifycenter implements the unified Notification Center.
//
// It aggregates signals from three sources — Alertmanager (firing
// alerts), the jobs subsystem (failed jobs), and the audit log
// (rejected outcomes) — into a single per-user-state-tracked event
// log. The Web GUI subscribes to a single SSE stream to drive the
// notification bell.
//
// The package is deliberately separate from internal/host/notify
// (which is the SMTP relay — outbound concern, completely different
// problem). Operators conflate the two; the API prefixes
// (/notifications/smtp vs /notifications/events) keep them distinct.
package notifycenter

import "time"

// Source is where an aggregated notification originated.
type Source string

const (
	SourceAlertmanager Source = "alertmanager"
	SourceJobs         Source = "jobs"
	SourceAudit        Source = "audit"
	SourceSystem       Source = "system"
)

// Valid reports whether s is a recognized Source.
func (s Source) Valid() bool {
	switch s {
	case SourceAlertmanager, SourceJobs, SourceAudit, SourceSystem:
		return true
	}
	return false
}

// Severity is the operator-visible importance class.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Valid reports whether v is a recognized Severity.
func (v Severity) Valid() bool {
	switch v {
	case SeverityInfo, SeverityWarning, SeverityCritical:
		return true
	}
	return false
}

// Event is the canonical notification record exposed to API clients.
// It mirrors the `notifications` row plus an embedded per-user state
// computed at read time.
type Event struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenantId,omitempty"`
	Source    Source    `json:"source"`
	SourceID  string    `json:"sourceId"`
	Severity  Severity  `json:"severity"`
	Title     string    `json:"title"`
	Body      string    `json:"body,omitempty"`
	Link      string    `json:"link,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UserState State     `json:"userState"`
}

// State is per-user notification state.
//
// Read, Dismissed, and Snoozed are convenience booleans computed from
// the timestamp fields:
//   - Read      == ReadAt is non-nil
//   - Dismissed == DismissedAt is non-nil
//   - Snoozed   == SnoozedUntil is non-nil AND > now()
//
// Both raw timestamps and the booleans are emitted so clients can
// either render them directly or compute richer states (e.g. "snoozed
// until 5pm").
type State struct {
	Read         bool       `json:"read"`
	Dismissed    bool       `json:"dismissed"`
	Snoozed      bool       `json:"snoozed"`
	ReadAt       *time.Time `json:"readAt,omitempty"`
	DismissedAt  *time.Time `json:"dismissedAt,omitempty"`
	SnoozedUntil *time.Time `json:"snoozedUntil,omitempty"`
}

// RecordInput is the shape passed to Manager.RecordEvent.
//
// Source + SourceID together form the dedup key: re-recording the
// same (Source, SourceID) is idempotent and returns the existing row.
type RecordInput struct {
	Source   Source
	SourceID string
	Severity Severity
	Title    string
	Body     string
	Link     string
	TenantID string
}
