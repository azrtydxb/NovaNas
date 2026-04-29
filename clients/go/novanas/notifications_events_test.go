package novanas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func newNCEClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSDK_ListNotificationEvents(t *testing.T) {
	want := NotificationEventList{
		Items: []NotificationEvent{{
			ID: "11111111-2222-3333-4444-555555555555",
			Source: NotificationSourceJobs, SourceID: "job-1",
			Severity: NotificationSeverityWarning, Title: "Job failed",
			CreatedAt: time.Now().UTC().Truncate(time.Second),
		}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/notifications/events", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("severity") != "warning" {
			t.Fatalf("missing severity filter: %q", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(want)
	})
	c := newNCEClient(t, mux)
	got, err := c.ListNotificationEvents(context.Background(), NotificationListOptions{
		Severity: NotificationSeverityWarning, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != want.Items[0].ID {
		t.Fatalf("wrong response: %+v", got)
	}
}

func TestSDK_UnreadCountAndStateMutations(t *testing.T) {
	mu := sync.Mutex{}
	calls := []string{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/notifications/events/unread-count", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]int64{"count": 7})
	})
	id := "11111111-2222-3333-4444-555555555555"
	mux.HandleFunc("/api/v1/notifications/events/"+id+"/read", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, "read")
		mu.Unlock()
		w.WriteHeader(200)
		fmt.Fprint(w, `{"status":"read"}`)
	})
	mux.HandleFunc("/api/v1/notifications/events/"+id+"/dismiss", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, "dismiss")
		mu.Unlock()
		w.WriteHeader(200)
		fmt.Fprint(w, `{"status":"dismissed"}`)
	})
	mux.HandleFunc("/api/v1/notifications/events/"+id+"/snooze", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, "snooze")
		mu.Unlock()
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["until"] == "" {
			t.Fatalf("missing until")
		}
		w.WriteHeader(200)
		fmt.Fprint(w, `{"status":"snoozed"}`)
	})
	mux.HandleFunc("/api/v1/notifications/events/read-all", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls = append(calls, "read-all")
		mu.Unlock()
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	c := newNCEClient(t, mux)
	ctx := context.Background()

	n, err := c.GetNotificationUnreadCount(ctx)
	if err != nil || n != 7 {
		t.Fatalf("unread count: n=%d err=%v", n, err)
	}
	if err := c.MarkNotificationRead(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := c.DismissNotification(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := c.SnoozeNotification(ctx, id, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := c.MarkAllNotificationsRead(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 4 {
		t.Fatalf("expected 4 calls, got %v", calls)
	}
}

func TestSDK_StreamNotificationEvents(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/notifications/events/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		ev := NotificationEvent{
			ID: "11111111-2222-3333-4444-555555555555",
			Source: NotificationSourceJobs, Severity: NotificationSeverityWarning,
			Title: "hi", CreatedAt: time.Now().UTC(),
		}
		buf, _ := json.Marshal(ev)
		fmt.Fprintf(w, "event: notification\ndata: %s\n\n", buf)
		flusher.Flush()
		// Allow client to consume then end.
		time.Sleep(100 * time.Millisecond)
	})
	c := newNCEClient(t, mux)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	gotID := make(chan string, 1)
	err := c.StreamNotificationEvents(ctx, func(ev NotificationEvent) {
		select {
		case gotID <- ev.ID:
		default:
		}
	})
	if err != nil {
		t.Logf("stream returned: %v", err)
	}
	select {
	case id := <-gotID:
		if id != "11111111-2222-3333-4444-555555555555" {
			t.Fatalf("wrong id %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no event observed")
	}
}
