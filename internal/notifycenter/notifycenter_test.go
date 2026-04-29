package notifycenter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// fakeQ is an in-memory implementation of the Queries interface used
// by the manager. It does not aim for SQL fidelity — only the
// behavior the manager observes.
type fakeQ struct {
	mu      sync.Mutex
	rows    []storedb.Notification
	bySrc   map[string]uuid.UUID // (source|source_id) -> id
	states  map[stateKey]storedb.NotificationState
	failOn  string // optional: name of method to fail
	failErr error
}

type stateKey struct {
	id   [16]byte
	user string
}

func newFakeQ() *fakeQ {
	return &fakeQ{
		bySrc:  make(map[string]uuid.UUID),
		states: make(map[stateKey]storedb.NotificationState),
	}
}

func (f *fakeQ) InsertNotification(_ context.Context, arg storedb.InsertNotificationParams) (storedb.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := arg.Source + "|" + arg.SourceID
	if existingID, ok := f.bySrc[key]; ok {
		for _, r := range f.rows {
			if r.ID.Bytes == existingID {
				return r, nil
			}
		}
	}
	id := uuid.UUID(arg.ID.Bytes)
	row := storedb.Notification{
		ID:        pgtype.UUID{Bytes: id, Valid: true},
		TenantID:  arg.TenantID,
		Source:    arg.Source,
		SourceID:  arg.SourceID,
		Severity:  arg.Severity,
		Title:     arg.Title,
		Body:      arg.Body,
		Link:      arg.Link,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	f.rows = append(f.rows, row)
	f.bySrc[key] = id
	return row, nil
}

func (f *fakeQ) GetNotification(_ context.Context, id pgtype.UUID) (storedb.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.ID.Bytes == id.Bytes {
			return r, nil
		}
	}
	return storedb.Notification{}, storedb.ErrNoRows
}

func (f *fakeQ) ListNotifications(_ context.Context, arg storedb.ListNotificationsParams) ([]storedb.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storedb.Notification, 0, len(f.rows))
	// reverse-chronological
	for i := len(f.rows) - 1; i >= 0; i-- {
		r := f.rows[i]
		if arg.Severity != nil && r.Severity != *arg.Severity {
			continue
		}
		if arg.Source != nil && r.Source != *arg.Source {
			continue
		}
		out = append(out, r)
		if arg.Lim > 0 && int32(len(out)) == arg.Lim {
			break
		}
	}
	return out, nil
}

func (f *fakeQ) GetUserState(_ context.Context, arg storedb.GetUserStateParams) (storedb.NotificationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := stateKey{id: arg.NotificationID.Bytes, user: arg.UserSubject}
	if s, ok := f.states[k]; ok {
		return s, nil
	}
	return storedb.NotificationState{}, storedb.ErrNoRows
}

func (f *fakeQ) ListUserStatesForNotifications(_ context.Context, arg storedb.ListUserStatesForNotificationsParams) ([]storedb.NotificationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storedb.NotificationState, 0, len(arg.Ids))
	for _, id := range arg.Ids {
		if s, ok := f.states[stateKey{id: id.Bytes, user: arg.UserSubject}]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeQ) UpsertUserStateRead(_ context.Context, arg storedb.UpsertUserStateReadParams) (storedb.NotificationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := stateKey{id: arg.NotificationID.Bytes, user: arg.UserSubject}
	s := f.states[k]
	s.NotificationID = arg.NotificationID
	s.UserSubject = arg.UserSubject
	if !s.ReadAt.Valid {
		s.ReadAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	f.states[k] = s
	return s, nil
}

func (f *fakeQ) UpsertUserStateDismiss(_ context.Context, arg storedb.UpsertUserStateDismissParams) (storedb.NotificationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := stateKey{id: arg.NotificationID.Bytes, user: arg.UserSubject}
	s := f.states[k]
	s.NotificationID = arg.NotificationID
	s.UserSubject = arg.UserSubject
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	s.DismissedAt = now
	if !s.ReadAt.Valid {
		s.ReadAt = now
	}
	f.states[k] = s
	return s, nil
}

func (f *fakeQ) UpsertUserStateSnooze(_ context.Context, arg storedb.UpsertUserStateSnoozeParams) (storedb.NotificationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := stateKey{id: arg.NotificationID.Bytes, user: arg.UserSubject}
	s := f.states[k]
	s.NotificationID = arg.NotificationID
	s.UserSubject = arg.UserSubject
	s.SnoozedUntil = arg.SnoozedUntil
	f.states[k] = s
	return s, nil
}

func (f *fakeQ) MarkAllReadForUser(_ context.Context, userSubject string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	for _, r := range f.rows {
		k := stateKey{id: r.ID.Bytes, user: userSubject}
		s := f.states[k]
		if s.DismissedAt.Valid {
			continue
		}
		s.NotificationID = r.ID
		s.UserSubject = userSubject
		if !s.ReadAt.Valid {
			s.ReadAt = now
		}
		f.states[k] = s
	}
	return nil
}

func (f *fakeQ) UnreadCountForUser(_ context.Context, userSubject string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	var n int64
	for _, r := range f.rows {
		k := stateKey{id: r.ID.Bytes, user: userSubject}
		s := f.states[k]
		if s.ReadAt.Valid || s.DismissedAt.Valid {
			continue
		}
		if s.SnoozedUntil.Valid && s.SnoozedUntil.Time.After(now) {
			continue
		}
		n++
	}
	return n, nil
}

// ---------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------

func TestRecordEventIdempotent(t *testing.T) {
	q := newFakeQ()
	m := NewManager(q, nil)
	in := RecordInput{
		Source: SourceJobs, SourceID: "job-1",
		Severity: SeverityWarning, Title: "Job failed",
	}
	a, err := m.RecordEvent(context.Background(), in)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	b, err := m.RecordEvent(context.Background(), in)
	if err != nil {
		t.Fatalf("record-2: %v", err)
	}
	if a.ID != b.ID {
		t.Fatalf("expected idempotent insert, got distinct ids %s vs %s", a.ID, b.ID)
	}
	if len(q.rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(q.rows))
	}
}

func TestRecordEventValidation(t *testing.T) {
	m := NewManager(newFakeQ(), nil)
	cases := []RecordInput{
		{Source: "bogus", SourceID: "x", Severity: SeverityInfo, Title: "t"},
		{Source: SourceJobs, SourceID: "x", Severity: "huge", Title: "t"},
		{Source: SourceJobs, SourceID: "", Severity: SeverityInfo, Title: "t"},
		{Source: SourceJobs, SourceID: "x", Severity: SeverityInfo, Title: ""},
	}
	for i, c := range cases {
		if _, err := m.RecordEvent(context.Background(), c); err == nil {
			t.Fatalf("case %d: expected error, got nil", i)
		}
	}
}

func TestListAndUserStateProjection(t *testing.T) {
	q := newFakeQ()
	m := NewManager(q, nil)
	ctx := context.Background()
	a, _ := m.RecordEvent(ctx, RecordInput{Source: SourceJobs, SourceID: "1", Severity: SeverityWarning, Title: "a"})
	b, _ := m.RecordEvent(ctx, RecordInput{Source: SourceAudit, SourceID: "2", Severity: SeverityInfo, Title: "b"})
	_, _ = m.RecordEvent(ctx, RecordInput{Source: SourceAlertmanager, SourceID: "3", Severity: SeverityCritical, Title: "c"})

	if err := m.MarkRead(ctx, "alice", a.ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if err := m.MarkDismissed(ctx, "alice", b.ID); err != nil {
		t.Fatalf("dismiss: %v", err)
	}

	all, err := m.ListForUser(ctx, "alice", ListFilter{Limit: 10, IncludeDismissed: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 events, got %d", len(all))
	}

	defaultView, _ := m.ListForUser(ctx, "alice", ListFilter{Limit: 10})
	if len(defaultView) != 2 {
		t.Fatalf("expected 2 events in default view (b dismissed), got %d", len(defaultView))
	}

	unreadOnly, _ := m.ListForUser(ctx, "alice", ListFilter{Limit: 10, OnlyUnread: true})
	if len(unreadOnly) != 1 {
		t.Fatalf("expected 1 unread event (only c), got %d", len(unreadOnly))
	}
	if unreadOnly[0].Severity != SeverityCritical {
		t.Fatalf("expected the critical alert as unread, got %s", unreadOnly[0].Severity)
	}

	count, _ := m.UnreadCountForUser(ctx, "alice")
	if count != 1 {
		t.Fatalf("expected unread count 1, got %d", count)
	}
}

func TestSnoozeHidesAndExpires(t *testing.T) {
	q := newFakeQ()
	m := NewManager(q, nil)
	ctx := context.Background()
	a, _ := m.RecordEvent(ctx, RecordInput{Source: SourceJobs, SourceID: "1", Severity: SeverityWarning, Title: "a"})

	until := time.Now().Add(1 * time.Hour)
	if err := m.Snooze(ctx, "bob", a.ID, until); err != nil {
		t.Fatalf("snooze: %v", err)
	}
	view, _ := m.ListForUser(ctx, "bob", ListFilter{Limit: 10})
	if len(view) != 0 {
		t.Fatalf("snoozed event should be hidden, got %d", len(view))
	}
	view, _ = m.ListForUser(ctx, "bob", ListFilter{Limit: 10, OnlySnoozed: true})
	if len(view) != 1 {
		t.Fatalf("OnlySnoozed should surface 1, got %d", len(view))
	}
	if !view[0].UserState.Snoozed {
		t.Fatalf("expected userState.snoozed=true")
	}

	// Snooze that has already expired must not hide the event.
	pastM := NewManager(newFakeQ(), nil)
	a2, _ := pastM.RecordEvent(ctx, RecordInput{Source: SourceJobs, SourceID: "1", Severity: SeverityWarning, Title: "a"})
	if err := pastM.Snooze(ctx, "bob", a2.ID, time.Now().Add(-1*time.Hour)); err != nil {
		t.Fatalf("snooze: %v", err)
	}
	view2, _ := pastM.ListForUser(ctx, "bob", ListFilter{Limit: 10})
	if len(view2) != 1 {
		t.Fatalf("expired snooze should re-surface the event, got %d", len(view2))
	}
	if view2[0].UserState.Snoozed {
		t.Fatalf("snoozed bool should be false for an expired snooze")
	}
}

func TestMarkAllReadIgnoresDismissed(t *testing.T) {
	q := newFakeQ()
	m := NewManager(q, nil)
	ctx := context.Background()
	a, _ := m.RecordEvent(ctx, RecordInput{Source: SourceJobs, SourceID: "1", Severity: SeverityWarning, Title: "a"})
	b, _ := m.RecordEvent(ctx, RecordInput{Source: SourceAudit, SourceID: "2", Severity: SeverityInfo, Title: "b"})
	_ = a
	_ = m.MarkDismissed(ctx, "alice", b.ID)

	if err := m.MarkAllRead(ctx, "alice"); err != nil {
		t.Fatalf("mark all: %v", err)
	}
	count, _ := m.UnreadCountForUser(ctx, "alice")
	if count != 0 {
		t.Fatalf("expected 0 unread after mark-all, got %d", count)
	}
}

func TestBusFanout(t *testing.T) {
	bus := NewBus()
	ch1, c1 := bus.Subscribe()
	ch2, c2 := bus.Subscribe()
	defer c1()
	defer c2()
	if bus.SubscriberCount() != 2 {
		t.Fatalf("expected 2 subs, got %d", bus.SubscriberCount())
	}
	ev := Event{ID: "x", Title: "hi"}
	bus.Publish(ev)
	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case got := <-ch:
			if got.ID != "x" {
				t.Fatalf("wrong event")
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber did not receive event")
		}
	}
}

func TestBusUnsubscribe(t *testing.T) {
	bus := NewBus()
	_, cancel := bus.Subscribe()
	if bus.SubscriberCount() != 1 {
		t.Fatalf("want 1, got %d", bus.SubscriberCount())
	}
	cancel()
	if bus.SubscriberCount() != 0 {
		t.Fatalf("want 0 after cancel, got %d", bus.SubscriberCount())
	}
	// Double-cancel must be safe.
	cancel()
}

func TestRecordEventPublishesToBus(t *testing.T) {
	m := NewManager(newFakeQ(), nil)
	ch, cancel := m.Subscribe()
	defer cancel()
	ev, err := m.RecordEvent(context.Background(), RecordInput{
		Source: SourceJobs, SourceID: "k", Severity: SeverityWarning, Title: "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-ch:
		if got.ID != ev.ID {
			t.Fatalf("wrong id")
		}
	case <-time.After(time.Second):
		t.Fatalf("no event on bus")
	}
}

func TestSeverityMapping(t *testing.T) {
	cases := map[string]Severity{
		"critical": SeverityCritical,
		"page":     SeverityCritical,
		"warning":  SeverityWarning,
		"info":     SeverityInfo,
		"":         SeverityInfo,
		"weird":    SeverityInfo,
	}
	for in, want := range cases {
		if got := mapAlertSeverity(in); got != want {
			t.Fatalf("mapAlertSeverity(%q)=%s, want %s", in, got, want)
		}
	}
}

func TestIsAuthFailure(t *testing.T) {
	yes := []string{"auth.login", "token_refresh", "user.login"}
	no := []string{"pool.create", "dataset.destroy"}
	for _, a := range yes {
		if !isAuthFailure(a) {
			t.Fatalf("expected %q to be auth failure", a)
		}
	}
	for _, a := range no {
		if isAuthFailure(a) {
			t.Fatalf("expected %q NOT to be auth failure", a)
		}
	}
}
