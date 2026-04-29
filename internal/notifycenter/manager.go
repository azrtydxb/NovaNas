package notifycenter

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// Queries is the subset of storedb.Queries the Manager needs. Defined
// here so tests can supply an in-memory fake without standing up a
// real Postgres.
type Queries interface {
	InsertNotification(ctx context.Context, arg storedb.InsertNotificationParams) (storedb.Notification, error)
	GetNotification(ctx context.Context, id pgtype.UUID) (storedb.Notification, error)
	ListNotifications(ctx context.Context, arg storedb.ListNotificationsParams) ([]storedb.Notification, error)
	GetUserState(ctx context.Context, arg storedb.GetUserStateParams) (storedb.NotificationState, error)
	ListUserStatesForNotifications(ctx context.Context, arg storedb.ListUserStatesForNotificationsParams) ([]storedb.NotificationState, error)
	UpsertUserStateRead(ctx context.Context, arg storedb.UpsertUserStateReadParams) (storedb.NotificationState, error)
	UpsertUserStateDismiss(ctx context.Context, arg storedb.UpsertUserStateDismissParams) (storedb.NotificationState, error)
	UpsertUserStateSnooze(ctx context.Context, arg storedb.UpsertUserStateSnoozeParams) (storedb.NotificationState, error)
	MarkAllReadForUser(ctx context.Context, userSubject string) error
	UnreadCountForUser(ctx context.Context, userSubject string) (int64, error)
}

// ListFilter narrows the result set for ListForUser.
//
// Severity and Source, when non-empty, must match exactly. The
// IncludeXxx flags compose: by default the list view excludes both
// dismissed and currently-snoozed entries (the operator-friendly
// default the bell drop-down should display). A "snoozed" filter view
// flips IncludeSnoozed=true AND OnlySnoozed=true.
type ListFilter struct {
	Severity         Severity
	Source           Source
	OnlyUnread       bool
	IncludeDismissed bool
	IncludeSnoozed   bool
	OnlySnoozed      bool
	Limit            int32
	CursorTime       time.Time
	CursorID         string
}

// Manager is the entry point for everything notification-center.
//
// It is safe for concurrent use. The SSE bus is owned here so a single
// RecordEvent both persists to Postgres AND fans out to currently-
// connected clients in one call.
type Manager struct {
	q      Queries
	bus    *Bus
	logger *slog.Logger
}

// NewManager builds a Manager. logger may be nil.
func NewManager(q Queries, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{q: q, bus: NewBus(), logger: logger}
}

// Bus exposes the underlying SSE fan-out for the HTTP handler. The
// Manager owns the bus lifecycle.
func (m *Manager) Bus() *Bus { return m.bus }

// RecordEvent persists a new notification (idempotent on
// Source+SourceID) and fans it out to subscribed SSE clients. The
// returned Event always has an empty UserState — per-user state is
// computed lazily in ListForUser / the SSE pump.
func (m *Manager) RecordEvent(ctx context.Context, in RecordInput) (Event, error) {
	if !in.Source.Valid() {
		return Event{}, errors.New("notifycenter: invalid source")
	}
	if !in.Severity.Valid() {
		return Event{}, errors.New("notifycenter: invalid severity")
	}
	if strings.TrimSpace(in.SourceID) == "" {
		return Event{}, errors.New("notifycenter: source_id is required")
	}
	if strings.TrimSpace(in.Title) == "" {
		return Event{}, errors.New("notifycenter: title is required")
	}
	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	var tenantPtr *string
	if t := strings.TrimSpace(in.TenantID); t != "" {
		tenantPtr = &t
	}
	row, err := m.q.InsertNotification(ctx, storedb.InsertNotificationParams{
		ID:       id,
		TenantID: tenantPtr,
		Source:   string(in.Source),
		SourceID: in.SourceID,
		Severity: string(in.Severity),
		Title:    in.Title,
		Body:     in.Body,
		Link:     in.Link,
	})
	if err != nil {
		return Event{}, err
	}
	ev := rowToEvent(row, nil)
	m.bus.Publish(ev)
	return ev, nil
}

// ListForUser returns the events visible to user, applying f.
//
// The default policy is conservative: dismissed events and currently
// snoozed events are excluded. Operators that want to recover a
// dismissed event must use the audit log (the underlying notification
// is preserved server-side; only that user's state hides it).
func (m *Manager) ListForUser(ctx context.Context, userSubject string, f ListFilter) ([]Event, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	params := storedb.ListNotificationsParams{Lim: f.Limit}
	if f.Severity != "" {
		s := string(f.Severity)
		params.Severity = &s
	}
	if f.Source != "" {
		s := string(f.Source)
		params.Source = &s
	}
	if !f.CursorTime.IsZero() {
		params.CursorTs = pgtype.Timestamptz{Time: f.CursorTime, Valid: true}
		if cid, err := uuid.Parse(f.CursorID); err == nil {
			params.CursorID = pgtype.UUID{Bytes: cid, Valid: true}
		}
	}
	rows, err := m.q.ListNotifications(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []Event{}, nil
	}
	ids := make([]pgtype.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	states, err := m.q.ListUserStatesForNotifications(ctx, storedb.ListUserStatesForNotificationsParams{
		UserSubject: userSubject,
		Ids:         ids,
	})
	if err != nil {
		return nil, err
	}
	stateMap := make(map[[16]byte]storedb.NotificationState, len(states))
	for _, s := range states {
		stateMap[s.NotificationID.Bytes] = s
	}
	now := time.Now()
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		st := stateMap[r.ID.Bytes]
		ev := rowToEvent(r, &st)
		if !applyFilter(ev, f, now) {
			continue
		}
		out = append(out, ev)
	}
	return out, nil
}

// applyFilter evaluates the in-memory filter that's awkward to express
// purely in SQL (read/dismissed/snoozed checks all live in a separate
// table and the joins-with-CASE expansion is harder to maintain than a
// short Go pass).
func applyFilter(ev Event, f ListFilter, now time.Time) bool {
	if f.OnlyUnread && ev.UserState.Read {
		return false
	}
	if !f.IncludeDismissed && ev.UserState.Dismissed {
		return false
	}
	snoozedNow := ev.UserState.SnoozedUntil != nil && ev.UserState.SnoozedUntil.After(now)
	if f.OnlySnoozed {
		return snoozedNow
	}
	if !f.IncludeSnoozed && snoozedNow {
		return false
	}
	return true
}

// UnreadCountForUser returns the bell-badge count.
func (m *Manager) UnreadCountForUser(ctx context.Context, userSubject string) (int64, error) {
	return m.q.UnreadCountForUser(ctx, userSubject)
}

// MarkRead marks a single event read for user. Idempotent.
func (m *Manager) MarkRead(ctx context.Context, userSubject string, id string) error {
	pgID, err := parsePGUUID(id)
	if err != nil {
		return err
	}
	_, err = m.q.UpsertUserStateRead(ctx, storedb.UpsertUserStateReadParams{
		NotificationID: pgID, UserSubject: userSubject,
	})
	return err
}

// MarkDismissed flags an event as dismissed for user. Dismissed events
// are also implicitly read.
func (m *Manager) MarkDismissed(ctx context.Context, userSubject string, id string) error {
	pgID, err := parsePGUUID(id)
	if err != nil {
		return err
	}
	_, err = m.q.UpsertUserStateDismiss(ctx, storedb.UpsertUserStateDismissParams{
		NotificationID: pgID, UserSubject: userSubject,
	})
	return err
}

// Snooze hides the event for user until `until`. A zero `until` clears
// the snooze.
func (m *Manager) Snooze(ctx context.Context, userSubject string, id string, until time.Time) error {
	pgID, err := parsePGUUID(id)
	if err != nil {
		return err
	}
	var ts pgtype.Timestamptz
	if !until.IsZero() {
		ts = pgtype.Timestamptz{Time: until, Valid: true}
	}
	_, err = m.q.UpsertUserStateSnooze(ctx, storedb.UpsertUserStateSnoozeParams{
		NotificationID: pgID, UserSubject: userSubject, SnoozedUntil: ts,
	})
	return err
}

// MarkAllRead marks every currently-unread, undismissed notification
// read for user. Used by the bell's "mark all read" affordance.
func (m *Manager) MarkAllRead(ctx context.Context, userSubject string) error {
	return m.q.MarkAllReadForUser(ctx, userSubject)
}

// Subscribe registers a SSE consumer. The returned channel emits
// Events as the bus fans them out. The caller MUST call the cleanup
// function on disconnect.
func (m *Manager) Subscribe() (<-chan Event, func()) {
	return m.bus.Subscribe()
}

// rowToEvent converts a DB row plus optional state into the public
// Event struct. state may be the zero value (all-zero NotificationID
// indicates "no state row"); in that case the State block is empty.
func rowToEvent(row storedb.Notification, state *storedb.NotificationState) Event {
	ev := Event{
		ID:        uuid.UUID(row.ID.Bytes).String(),
		Source:    Source(row.Source),
		SourceID:  row.SourceID,
		Severity:  Severity(row.Severity),
		Title:     row.Title,
		Body:      row.Body,
		Link:      row.Link,
		CreatedAt: row.CreatedAt.Time,
	}
	if row.TenantID != nil {
		ev.TenantID = *row.TenantID
	}
	if state != nil && state.NotificationID.Valid {
		now := time.Now()
		if state.ReadAt.Valid {
			t := state.ReadAt.Time
			ev.UserState.ReadAt = &t
			ev.UserState.Read = true
		}
		if state.DismissedAt.Valid {
			t := state.DismissedAt.Time
			ev.UserState.DismissedAt = &t
			ev.UserState.Dismissed = true
		}
		if state.SnoozedUntil.Valid {
			t := state.SnoozedUntil.Time
			ev.UserState.SnoozedUntil = &t
			ev.UserState.Snoozed = t.After(now)
		}
	}
	return ev
}

func parsePGUUID(s string) (pgtype.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, errors.New("notifycenter: invalid notification id")
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}
