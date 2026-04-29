package novanas

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// NotificationSource and NotificationSeverity are the exposed enums
// for the unified Notification Center.
type NotificationSource string

const (
	NotificationSourceAlertmanager NotificationSource = "alertmanager"
	NotificationSourceJobs         NotificationSource = "jobs"
	NotificationSourceAudit        NotificationSource = "audit"
	NotificationSourceSystem       NotificationSource = "system"
)

type NotificationSeverity string

const (
	NotificationSeverityInfo     NotificationSeverity = "info"
	NotificationSeverityWarning  NotificationSeverity = "warning"
	NotificationSeverityCritical NotificationSeverity = "critical"
)

// NotificationEvent mirrors notifycenter.Event on the server.
type NotificationEvent struct {
	ID        string                `json:"id"`
	TenantID  string                `json:"tenantId,omitempty"`
	Source    NotificationSource    `json:"source"`
	SourceID  string                `json:"sourceId"`
	Severity  NotificationSeverity  `json:"severity"`
	Title     string                `json:"title"`
	Body      string                `json:"body,omitempty"`
	Link      string                `json:"link,omitempty"`
	CreatedAt time.Time             `json:"createdAt"`
	UserState NotificationUserState `json:"userState"`
}

// NotificationUserState is the per-user state attached to each event.
type NotificationUserState struct {
	Read         bool       `json:"read"`
	Dismissed    bool       `json:"dismissed"`
	Snoozed      bool       `json:"snoozed"`
	ReadAt       *time.Time `json:"readAt,omitempty"`
	DismissedAt  *time.Time `json:"dismissedAt,omitempty"`
	SnoozedUntil *time.Time `json:"snoozedUntil,omitempty"`
}

// NotificationEventList is the response envelope of the list endpoint.
type NotificationEventList struct {
	Items      []NotificationEvent `json:"items"`
	NextCursor string              `json:"nextCursor,omitempty"`
}

// NotificationListOptions filters the list endpoint.
type NotificationListOptions struct {
	Severity         NotificationSeverity
	Source           NotificationSource
	OnlyUnread       bool
	IncludeDismissed bool
	IncludeSnoozed   bool
	OnlySnoozed      bool
	Limit            int
	Cursor           string
}

// ListNotificationEvents calls GET /notifications/events.
func (c *Client) ListNotificationEvents(ctx context.Context, opts NotificationListOptions) (*NotificationEventList, error) {
	q := url.Values{}
	if opts.Severity != "" {
		q.Set("severity", string(opts.Severity))
	}
	if opts.Source != "" {
		q.Set("source", string(opts.Source))
	}
	if opts.OnlyUnread {
		q.Set("unread", "true")
	}
	if opts.IncludeDismissed {
		q.Set("includeDismissed", "true")
	}
	if opts.IncludeSnoozed {
		q.Set("includeSnoozed", "true")
	}
	if opts.OnlySnoozed {
		q.Set("onlySnoozed", "true")
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Cursor != "" {
		q.Set("cursor", opts.Cursor)
	}
	var out NotificationEventList
	if _, err := c.do(ctx, http.MethodGet, "/notifications/events", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetNotificationUnreadCount calls GET /notifications/events/unread-count.
func (c *Client) GetNotificationUnreadCount(ctx context.Context) (int64, error) {
	var out struct {
		Count int64 `json:"count"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/notifications/events/unread-count", nil, nil, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

// MarkNotificationRead calls POST /notifications/events/{id}/read.
func (c *Client) MarkNotificationRead(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("novanas: id is required")
	}
	_, err := c.do(ctx, http.MethodPost, "/notifications/events/"+url.PathEscape(id)+"/read", nil, nil, nil)
	return err
}

// DismissNotification calls POST /notifications/events/{id}/dismiss.
func (c *Client) DismissNotification(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("novanas: id is required")
	}
	_, err := c.do(ctx, http.MethodPost, "/notifications/events/"+url.PathEscape(id)+"/dismiss", nil, nil, nil)
	return err
}

// SnoozeNotification calls POST /notifications/events/{id}/snooze.
func (c *Client) SnoozeNotification(ctx context.Context, id string, until time.Time) error {
	if id == "" {
		return errors.New("novanas: id is required")
	}
	if until.IsZero() {
		return errors.New("novanas: until is required")
	}
	body := map[string]string{"until": until.UTC().Format(time.RFC3339Nano)}
	_, err := c.do(ctx, http.MethodPost, "/notifications/events/"+url.PathEscape(id)+"/snooze", nil, body, nil)
	return err
}

// MarkAllNotificationsRead calls POST /notifications/events/read-all.
func (c *Client) MarkAllNotificationsRead(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodPost, "/notifications/events/read-all", nil, nil, nil)
	return err
}

// StreamNotificationEvents subscribes to the SSE event stream and
// invokes onEvent for each "event: notification" frame received.
// Returns when ctx is cancelled or the underlying connection closes.
//
// Heartbeats (`: keepalive`, `: connected`) are silently consumed.
// The function decodes each event payload into NotificationEvent;
// callers that want raw frames should call the HTTP endpoint
// directly.
func (c *Client) StreamNotificationEvents(ctx context.Context, onEvent func(NotificationEvent)) error {
	if onEvent == nil {
		return errors.New("novanas: onEvent is required")
	}
	u := c.BaseURL + apiPrefix + "/notifications/events/stream"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if tok := c.token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	} else {
		req.Header.Set("User-Agent", DefaultUserAgent)
	}
	// Use a fresh client without a response timeout so the long-lived
	// stream isn't terminated by HTTPClient.Timeout.
	httpc := &http.Client{Transport: c.HTTPClient.Transport}
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("novanas: stream returned %d: %s", resp.StatusCode, strings.TrimSpace(string(buf)))
	}
	br := bufio.NewReader(resp.Body)
	var frame strings.Builder
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		if line == "\n" || line == "\r\n" {
			parseFrame(frame.String(), onEvent)
			frame.Reset()
			continue
		}
		frame.WriteString(line)
	}
}

func parseFrame(frame string, cb func(NotificationEvent)) {
	if !strings.Contains(frame, "event: notification") {
		return
	}
	for _, line := range strings.Split(frame, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		payload = strings.TrimSpace(payload)
		if payload == "" {
			continue
		}
		var ev NotificationEvent
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		cb(ev)
	}
}
