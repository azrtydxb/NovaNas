package novanas

import (
	"bufio"
	"context"
	"encoding/csv"
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

// AuditEvent mirrors the AuditEvent schema in api/openapi.yaml.
//
// Field names follow the brief vocabulary (resource/outcome). Payload is
// the raw, server-redacted request body (when JSON); it's left as
// json.RawMessage so callers can decode it into their own struct.
type AuditEvent struct {
	ID        int64           `json:"id"`
	Timestamp time.Time       `json:"timestamp"`
	Actor     string          `json:"actor,omitempty"`
	Action    string          `json:"action"`
	Resource  string          `json:"resource"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Outcome   string          `json:"outcome"`
}

// AuditListResponse mirrors AuditListResponse in api/openapi.yaml.
type AuditListResponse struct {
	Items      []AuditEvent `json:"items"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

// AuditSummaryRow mirrors AuditSummaryRow in api/openapi.yaml.
type AuditSummaryRow struct {
	Actor   string `json:"actor"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
	Count   int64  `json:"count"`
}

// AuditFilter is the query-param bundle accepted by ListAudit, ExportAudit,
// and (since/until only) SummarizeAudit.
//
// Zero values mean "no filter". SourceIP accepts a bare IP or a CIDR.
type AuditFilter struct {
	Actor    string
	Action   string
	Resource string // prefix-matched server-side
	Outcome  string // "accepted" or "rejected"
	Since    time.Time
	Until    time.Time
	SourceIP string
}

// values projects the filter into an url.Values. Empty fields are
// omitted entirely so the server applies defaults.
func (f AuditFilter) values() url.Values {
	v := url.Values{}
	if f.Actor != "" {
		v.Set("actor", f.Actor)
	}
	if f.Action != "" {
		v.Set("action", f.Action)
	}
	if f.Resource != "" {
		v.Set("resource", f.Resource)
	}
	if f.Outcome != "" {
		v.Set("outcome", f.Outcome)
	}
	if !f.Since.IsZero() {
		v.Set("since", f.Since.UTC().Format(time.RFC3339Nano))
	}
	if !f.Until.IsZero() {
		v.Set("until", f.Until.UTC().Format(time.RFC3339Nano))
	}
	if f.SourceIP != "" {
		v.Set("source_ip", f.SourceIP)
	}
	return v
}

// ListAudit fetches one cursor-paginated page of audit events.
//
// Pass cursor="" for the first page; on subsequent calls pass the
// `NextCursor` field of the previous response. limit may be 0 to use the
// server default (100). Server max is 1000.
func (c *Client) ListAudit(ctx context.Context, f AuditFilter, cursor string, limit int) (*AuditListResponse, error) {
	q := f.values()
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out AuditListResponse
	if _, err := c.do(ctx, http.MethodGet, "/audit", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// IterateAudit walks every page returned by /audit and invokes fn on each
// event. Stops early if fn returns a non-nil error or the server returns
// no more rows. The caller can cancel via ctx.
func (c *Client) IterateAudit(ctx context.Context, f AuditFilter, pageSize int, fn func(AuditEvent) error) error {
	cursor := ""
	for {
		page, err := c.ListAudit(ctx, f, cursor, pageSize)
		if err != nil {
			return err
		}
		for _, ev := range page.Items {
			if err := fn(ev); err != nil {
				return err
			}
		}
		if page.NextCursor == "" || len(page.Items) == 0 {
			return nil
		}
		cursor = page.NextCursor
	}
}

// SummarizeAudit returns aggregate counts grouped by (actor, action,
// outcome) within an optional time window. Filter fields other than
// Since/Until are ignored.
func (c *Client) SummarizeAudit(ctx context.Context, since, until time.Time) ([]AuditSummaryRow, error) {
	q := url.Values{}
	if !since.IsZero() {
		q.Set("since", since.UTC().Format(time.RFC3339Nano))
	}
	if !until.IsZero() {
		q.Set("until", until.UTC().Format(time.RFC3339Nano))
	}
	var out []AuditSummaryRow
	if _, err := c.do(ctx, http.MethodGet, "/audit/summary", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AuditExportFormat selects the wire format of the streaming export.
type AuditExportFormat string

const (
	AuditExportCSV   AuditExportFormat = "csv"
	AuditExportJSONL AuditExportFormat = "jsonl"
)

// ErrAuditExportBusy is returned by ExportAudit when the server reports
// 429 (another export is already running for this user).
var ErrAuditExportBusy = errors.New("novanas: audit export already in progress for this user")

// ExportAudit calls /audit/export and streams events to fn one at a time.
//
// The implementation never buffers the full response in memory: it
// drains the HTTP body row-by-row via a CSV reader (for csv) or a
// line-buffered JSON decoder (for jsonl), invoking fn after every row.
// fn returning a non-nil error aborts the read and closes the stream.
//
// The bearer token is sent automatically (same as every other call).
// Cancel ctx to abort the download — the server detects the disconnect
// and stops paginating its DB query.
func (c *Client) ExportAudit(ctx context.Context, format AuditExportFormat, f AuditFilter, fn func(AuditEvent) error) error {
	if format == "" {
		format = AuditExportCSV
	}
	if format != AuditExportCSV && format != AuditExportJSONL {
		return fmt.Errorf("novanas: invalid export format %q", format)
	}
	if fn == nil {
		return errors.New("novanas: ExportAudit requires a non-nil callback")
	}

	q := f.values()
	q.Set("format", string(format))
	u := c.BaseURL + apiPrefix + "/audit/export?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", string(audioAcceptHeader(format)))
	if ua := c.UserAgent; ua != "" {
		req.Header.Set("User-Agent", ua)
	} else {
		req.Header.Set("User-Agent", DefaultUserAgent)
	}
	if tok := c.token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		// Drain & discard; the body is the standard error envelope.
		_, _ = io.Copy(io.Discard, resp.Body)
		return ErrAuditExportBusy
	}
	if resp.StatusCode >= 400 {
		buf, _ := io.ReadAll(resp.Body)
		var env struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(buf, &env)
		return &APIError{StatusCode: resp.StatusCode, Code: env.Error, Message: env.Message}
	}

	switch format {
	case AuditExportCSV:
		return streamCSV(resp.Body, fn)
	case AuditExportJSONL:
		return streamJSONL(resp.Body, fn)
	}
	return nil
}

// audioAcceptHeader returns the right Accept value for the format.
func audioAcceptHeader(f AuditExportFormat) string {
	if f == AuditExportJSONL {
		return "application/x-ndjson"
	}
	return "text/csv"
}

// streamCSV reads a CSV body produced by /audit/export?format=csv.
//
// The header row is read once to map column names to indices. Rows are
// decoded one at a time via csv.Reader.Read so memory stays bounded by
// a single row regardless of the total size of the export.
func streamCSV(body io.Reader, fn func(AuditEvent) error) error {
	r := csv.NewReader(body)
	r.FieldsPerRecord = -1 // tolerate trailing-quote edge cases
	header, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("novanas: read csv header: %w", err)
	}
	idx := map[string]int{}
	for i, name := range header {
		idx[name] = i
	}
	get := func(row []string, name string) string {
		i, ok := idx[name]
		if !ok || i >= len(row) {
			return ""
		}
		return row[i]
	}
	for {
		row, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("novanas: read csv row: %w", err)
		}
		ev := AuditEvent{
			Action:    get(row, "action"),
			Resource:  get(row, "resource"),
			Actor:     get(row, "actor"),
			Outcome:   get(row, "outcome"),
			RequestID: get(row, "request_id"),
		}
		if v := get(row, "id"); v != "" {
			ev.ID, _ = strconv.ParseInt(v, 10, 64)
		}
		if v := get(row, "timestamp"); v != "" {
			ev.Timestamp, _ = time.Parse(time.RFC3339Nano, v)
		}
		if v := get(row, "payload"); v != "" {
			ev.Payload = json.RawMessage(v)
		}
		if err := fn(ev); err != nil {
			return err
		}
	}
}

// streamJSONL reads a newline-delimited JSON body. We use bufio.Scanner
// with a generous buffer cap (8 MiB) so a single oversized payload row
// doesn't break the stream.
func streamJSONL(body io.Reader, fn func(AuditEvent) error) error {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev AuditEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return fmt.Errorf("novanas: decode jsonl: %w", err)
		}
		if err := fn(ev); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("novanas: read jsonl body: %w", err)
	}
	return nil
}
