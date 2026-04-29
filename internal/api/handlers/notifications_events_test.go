package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
	"github.com/novanas/nova-nas/internal/notifycenter"
)

// nceFakeQ is a tiny in-memory implementation of notifycenter.Queries
// for HTTP-handler tests. It mirrors the manager-level fake but is
// scoped here to avoid importing test files across packages.
type nceFakeQ struct {
	mu     sync.Mutex
	rows   []storedb.Notification
	bySrc  map[string]uuid.UUID
	states map[nceStateKey]storedb.NotificationState
}

type nceStateKey struct {
	id   [16]byte
	user string
}

func newNCEFakeQ() *nceFakeQ {
	return &nceFakeQ{
		bySrc:  make(map[string]uuid.UUID),
		states: make(map[nceStateKey]storedb.NotificationState),
	}
}

func (f *nceFakeQ) InsertNotification(_ context.Context, arg storedb.InsertNotificationParams) (storedb.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := arg.Source + "|" + arg.SourceID
	if existing, ok := f.bySrc[key]; ok {
		for _, r := range f.rows {
			if r.ID.Bytes == existing {
				return r, nil
			}
		}
	}
	id := uuid.UUID(arg.ID.Bytes)
	row := storedb.Notification{
		ID: pgtype.UUID{Bytes: id, Valid: true}, TenantID: arg.TenantID,
		Source: arg.Source, SourceID: arg.SourceID, Severity: arg.Severity,
		Title: arg.Title, Body: arg.Body, Link: arg.Link,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	f.rows = append(f.rows, row)
	f.bySrc[key] = id
	return row, nil
}

func (f *nceFakeQ) GetNotification(_ context.Context, id pgtype.UUID) (storedb.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.ID.Bytes == id.Bytes {
			return r, nil
		}
	}
	return storedb.Notification{}, storedb.ErrNoRows
}

func (f *nceFakeQ) ListNotifications(_ context.Context, arg storedb.ListNotificationsParams) ([]storedb.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storedb.Notification, 0, len(f.rows))
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

func (f *nceFakeQ) GetUserState(_ context.Context, arg storedb.GetUserStateParams) (storedb.NotificationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.states[nceStateKey{id: arg.NotificationID.Bytes, user: arg.UserSubject}]; ok {
		return s, nil
	}
	return storedb.NotificationState{}, storedb.ErrNoRows
}

func (f *nceFakeQ) ListUserStatesForNotifications(_ context.Context, arg storedb.ListUserStatesForNotificationsParams) ([]storedb.NotificationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []storedb.NotificationState{}
	for _, id := range arg.Ids {
		if s, ok := f.states[nceStateKey{id: id.Bytes, user: arg.UserSubject}]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *nceFakeQ) UpsertUserStateRead(_ context.Context, arg storedb.UpsertUserStateReadParams) (storedb.NotificationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := nceStateKey{id: arg.NotificationID.Bytes, user: arg.UserSubject}
	s := f.states[k]
	s.NotificationID = arg.NotificationID
	s.UserSubject = arg.UserSubject
	if !s.ReadAt.Valid {
		s.ReadAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	f.states[k] = s
	return s, nil
}

func (f *nceFakeQ) UpsertUserStateDismiss(_ context.Context, arg storedb.UpsertUserStateDismissParams) (storedb.NotificationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := nceStateKey{id: arg.NotificationID.Bytes, user: arg.UserSubject}
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

func (f *nceFakeQ) UpsertUserStateSnooze(_ context.Context, arg storedb.UpsertUserStateSnoozeParams) (storedb.NotificationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := nceStateKey{id: arg.NotificationID.Bytes, user: arg.UserSubject}
	s := f.states[k]
	s.NotificationID = arg.NotificationID
	s.UserSubject = arg.UserSubject
	s.SnoozedUntil = arg.SnoozedUntil
	f.states[k] = s
	return s, nil
}

func (f *nceFakeQ) MarkAllReadForUser(_ context.Context, user string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	for _, r := range f.rows {
		k := nceStateKey{id: r.ID.Bytes, user: user}
		s := f.states[k]
		if s.DismissedAt.Valid {
			continue
		}
		s.NotificationID = r.ID
		s.UserSubject = user
		if !s.ReadAt.Valid {
			s.ReadAt = now
		}
		f.states[k] = s
	}
	return nil
}

func (f *nceFakeQ) UnreadCountForUser(_ context.Context, user string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	var n int64
	for _, r := range f.rows {
		s := f.states[nceStateKey{id: r.ID.Bytes, user: user}]
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

// ----- Test helpers ----------------------------------------------------

func nceNewServer(t *testing.T) (*httptest.Server, *notifycenter.Manager) {
	t.Helper()
	q := newNCEFakeQ()
	mgr := notifycenter.NewManager(q, nil)
	h := &NotificationsEventsHandler{Mgr: mgr}
	r := chi.NewRouter()
	r.Get("/notifications/events", h.List)
	r.Get("/notifications/events/unread-count", h.UnreadCount)
	r.Get("/notifications/events/stream", h.Stream)
	r.Post("/notifications/events/{id}/read", h.MarkRead)
	r.Post("/notifications/events/{id}/dismiss", h.MarkDismissed)
	r.Post("/notifications/events/{id}/snooze", h.Snooze)
	r.Post("/notifications/events/read-all", h.MarkAllRead)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, mgr
}

// ----- Tests -----------------------------------------------------------

func TestNotificationsEvents_ListAndUnreadCount(t *testing.T) {
	srv, mgr := nceNewServer(t)
	ctx := context.Background()
	if _, err := mgr.RecordEvent(ctx, notifycenter.RecordInput{
		Source: notifycenter.SourceJobs, SourceID: "1",
		Severity: notifycenter.SeverityWarning, Title: "Job failed",
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(srv.URL + "/notifications/events?limit=10")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var body struct {
		Items []notifycenter.Event `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(body.Items))
	}
	if body.Items[0].Severity != notifycenter.SeverityWarning {
		t.Fatalf("wrong severity")
	}

	resp2, err := http.Get(srv.URL + "/notifications/events/unread-count")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var c struct{ Count int64 }
	if err := json.NewDecoder(resp2.Body).Decode(&c); err != nil {
		t.Fatal(err)
	}
	if c.Count != 1 {
		t.Fatalf("want unread count 1, got %d", c.Count)
	}
}

func TestNotificationsEvents_BadFilter(t *testing.T) {
	srv, _ := nceNewServer(t)
	resp, err := http.Get(srv.URL + "/notifications/events?severity=cataclysmic")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestNotificationsEvents_MarkReadDismissSnooze(t *testing.T) {
	srv, mgr := nceNewServer(t)
	ctx := context.Background()
	ev, _ := mgr.RecordEvent(ctx, notifycenter.RecordInput{
		Source: notifycenter.SourceJobs, SourceID: "1",
		Severity: notifycenter.SeverityWarning, Title: "x",
	})

	for _, op := range []string{"read", "dismiss"} {
		resp, err := http.Post(srv.URL+"/notifications/events/"+ev.ID+"/"+op, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("op %s: status %d", op, resp.StatusCode)
		}
	}

	body := bytes.NewBufferString(fmt.Sprintf(`{"until":"%s"}`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)))
	resp, err := http.Post(srv.URL+"/notifications/events/"+ev.ID+"/snooze", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("snooze status %d", resp.StatusCode)
	}

	// Bad UUID
	resp, err = http.Post(srv.URL+"/notifications/events/not-a-uuid/read", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad id, got %d", resp.StatusCode)
	}
}

func TestNotificationsEvents_MarkAllRead(t *testing.T) {
	srv, mgr := nceNewServer(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, _ = mgr.RecordEvent(ctx, notifycenter.RecordInput{
			Source: notifycenter.SourceJobs, SourceID: fmt.Sprintf("%d", i),
			Severity: notifycenter.SeverityWarning, Title: "x",
		})
	}
	resp, err := http.Post(srv.URL+"/notifications/events/read-all", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	resp2, err := http.Get(srv.URL + "/notifications/events/unread-count")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var c struct{ Count int64 }
	_ = json.NewDecoder(resp2.Body).Decode(&c)
	if c.Count != 0 {
		t.Fatalf("expected 0 unread after read-all, got %d", c.Count)
	}
}

func TestNotificationsEvents_SSEStream(t *testing.T) {
	srv, mgr := nceNewServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/notifications/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content-type=%q", got)
	}

	br := bufio.NewReader(resp.Body)
	// First frame is the connect heartbeat.
	first, err := readSSEFrame(br)
	if err != nil {
		t.Fatalf("read connect frame: %v", err)
	}
	if !strings.Contains(first, ": connected") {
		t.Fatalf("expected connect heartbeat, got %q", first)
	}

	// Give the SSE goroutine a moment to register the subscription.
	deadline := time.Now().Add(2 * time.Second)
	for mgr.Bus().SubscriberCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	// Publish an event; expect the SSE stream to emit it.
	ev, err := mgr.RecordEvent(context.Background(), notifycenter.RecordInput{
		Source: notifycenter.SourceJobs, SourceID: "1",
		Severity: notifycenter.SeverityWarning, Title: "Job failed",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Read frames until we see the notification.
	gotEvent := false
	timeout := time.After(3 * time.Second)
loop:
	for {
		select {
		case <-timeout:
			t.Fatalf("timed out waiting for notification SSE event")
		default:
		}
		frame, err := readSSEFrame(br)
		if err != nil {
			t.Fatalf("read frame: %v", err)
		}
		if !strings.Contains(frame, "event: notification") {
			continue
		}
		// Extract data line and decode JSON.
		for _, line := range strings.Split(frame, "\n") {
			if strings.HasPrefix(line, "data: ") {
				var got notifycenter.Event
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &got); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if got.ID != ev.ID {
					t.Fatalf("wrong id %s vs %s", got.ID, ev.ID)
				}
				gotEvent = true
				break loop
			}
		}
	}
	if !gotEvent {
		t.Fatalf("no notification event observed")
	}
}

// readSSEFrame returns one SSE frame (everything up to and including
// the terminating blank line).
func readSSEFrame(br *bufio.Reader) (string, error) {
	var buf bytes.Buffer
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF && buf.Len() > 0 {
				return buf.String(), nil
			}
			return "", err
		}
		buf.WriteString(line)
		if line == "\n" || line == "\r\n" {
			return buf.String(), nil
		}
	}
}
