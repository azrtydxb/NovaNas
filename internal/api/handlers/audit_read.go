// Audit read & export API.
//
// The audit_log table is written by the Audit middleware on every
// state-changing request (see internal/api/middleware/audit.go). This
// handler exposes the rows for read by operators with PermAuditRead.
//
// Schema reality (see internal/store/migrations/0001_init.sql): rows have
// (id, ts, actor, action, target, request_id, payload, result). The task
// brief refers to them as resource/outcome/etc.; the API surface uses the
// brief's vocabulary and maps onto the existing columns:
//
//	resource <- target  (prefix-matched)
//	outcome  <- result  (accepted|rejected)
//
// source_ip and client_id do not exist as columns. The source_ip filter,
// when supplied, is matched against payload->>'source_ip' as a CIDR after
// the rows are streamed back from the DB. Rows whose payload lacks a
// source_ip key are excluded by the filter.
package handlers

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/auth"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// AuditReadQ is the subset of the sqlc Querier the handler needs.
// Wired with *storedb.Queries in production; faked in tests.
type AuditReadQ interface {
	SearchAudit(ctx context.Context, p storedb.SearchAuditParams) ([]storedb.AuditLog, error)
	SummaryAudit(ctx context.Context, p storedb.SummaryAuditParams) ([]storedb.SummaryAuditRow, error)
	InsertAudit(ctx context.Context, p storedb.InsertAuditParams) error
}

// AuditReadHandler serves /audit/* read endpoints.
type AuditReadHandler struct {
	Logger *slog.Logger
	Q      AuditReadQ

	// exportLocks tracks which actors currently have an export in flight,
	// so the per-user "one concurrent export" rule is enforced even across
	// multiple goroutines/clients.
	exportLocks sync.Map // key: actor string, val: struct{}
}

const (
	auditDefaultPageSize = 100
	auditMaxPageSize     = 1000
)

// AuditEvent is the wire shape returned by /audit and /audit/export.
//
// Field names use the brief's vocabulary (resource/outcome) but the
// values come straight from the DB columns (target/result).
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

// AuditListResponse wraps a page plus a next cursor.
type AuditListResponse struct {
	Items      []AuditEvent `json:"items"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

// AuditSummaryRow is one row of the summary aggregation.
type AuditSummaryRow struct {
	Actor   string `json:"actor"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
	Count   int64  `json:"count"`
}

// auditFilter holds the parsed, validated query params shared by /audit
// and /audit/export.
type auditFilter struct {
	actor    *string
	action   *string
	outcome  *string
	resource *string // prefix
	since    pgtype.Timestamptz
	until    pgtype.Timestamptz
	srcCIDR  *net.IPNet // post-filtered against payload->>'source_ip'
	pageSize int32
	cursorTs pgtype.Timestamptz
	cursorID *int64
}

// rowToEvent converts a raw DB row to the wire format.
func rowToEvent(r storedb.AuditLog) AuditEvent {
	ev := AuditEvent{
		ID:        r.ID,
		Action:    r.Action,
		Resource:  r.Target,
		RequestID: r.RequestID,
		Outcome:   r.Result,
	}
	if r.Ts.Valid {
		ev.Timestamp = r.Ts.Time.UTC()
	}
	if r.Actor != nil {
		ev.Actor = *r.Actor
	}
	if len(r.Payload) > 0 {
		ev.Payload = json.RawMessage(r.Payload)
	}
	return ev
}

// matchesSourceIP returns true if the row's payload contains source_ip and
// the IP falls inside cidr. Rows without source_ip are excluded.
func matchesSourceIP(r storedb.AuditLog, cidr *net.IPNet) bool {
	if cidr == nil {
		return true
	}
	if len(r.Payload) == 0 {
		return false
	}
	var p map[string]json.RawMessage
	if err := json.Unmarshal(r.Payload, &p); err != nil {
		return false
	}
	raw, ok := p["source_ip"]
	if !ok {
		return false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return false
	}
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil {
		return false
	}
	return cidr.Contains(ip)
}

// encodeCursor turns (ts, id) into the opaque base64 token returned to
// clients as next_cursor.
func encodeCursor(ts time.Time, id int64) string {
	raw := fmt.Sprintf("%d:%d", ts.UnixNano(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor parses a token previously produced by encodeCursor. Empty
// input returns zero values without error.
func decodeCursor(tok string) (pgtype.Timestamptz, *int64, error) {
	var ts pgtype.Timestamptz
	if tok == "" {
		return ts, nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return ts, nil, fmt.Errorf("malformed cursor")
	}
	parts := strings.SplitN(string(b), ":", 2)
	if len(parts) != 2 {
		return ts, nil, fmt.Errorf("malformed cursor")
	}
	ns, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ts, nil, fmt.Errorf("malformed cursor")
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return ts, nil, fmt.Errorf("malformed cursor")
	}
	ts = pgtype.Timestamptz{Time: time.Unix(0, ns).UTC(), Valid: true}
	return ts, &id, nil
}

// parseFilter validates query params and returns a filter or a typed
// error message suitable for a 400 response.
func parseFilter(q map[string][]string, allowCursor bool) (*auditFilter, string) {
	f := &auditFilter{pageSize: auditDefaultPageSize}

	pick := func(name string) *string {
		vals, ok := q[name]
		if !ok || len(vals) == 0 || vals[0] == "" {
			return nil
		}
		v := vals[0]
		return &v
	}

	f.actor = pick("actor")
	f.action = pick("action")
	f.outcome = pick("outcome")
	f.resource = pick("resource")
	if f.outcome != nil && *f.outcome != "accepted" && *f.outcome != "rejected" {
		return nil, "outcome must be accepted or rejected"
	}

	parseTS := func(name string) (pgtype.Timestamptz, string) {
		raw := pick(name)
		if raw == nil {
			return pgtype.Timestamptz{}, ""
		}
		t, err := time.Parse(time.RFC3339Nano, *raw)
		if err != nil {
			t, err = time.Parse(time.RFC3339, *raw)
		}
		if err != nil {
			return pgtype.Timestamptz{}, name + " must be RFC3339"
		}
		return pgtype.Timestamptz{Time: t.UTC(), Valid: true}, ""
	}

	var msg string
	if f.since, msg = parseTS("since"); msg != "" {
		return nil, msg
	}
	if f.until, msg = parseTS("until"); msg != "" {
		return nil, msg
	}
	if f.since.Valid && f.until.Valid && !f.until.Time.After(f.since.Time) {
		return nil, "until must be after since"
	}

	if raw := pick("source_ip"); raw != nil {
		v := strings.TrimSpace(*raw)
		// Accept bare IPs by appending /32 or /128.
		if !strings.Contains(v, "/") {
			if ip := net.ParseIP(v); ip != nil {
				if ip.To4() != nil {
					v += "/32"
				} else {
					v += "/128"
				}
			}
		}
		_, n, err := net.ParseCIDR(v)
		if err != nil {
			return nil, "source_ip must be a valid IP or CIDR"
		}
		f.srcCIDR = n
	}

	if raw := pick("limit"); raw != nil {
		v, err := strconv.Atoi(*raw)
		if err != nil || v <= 0 {
			return nil, "limit must be a positive integer"
		}
		if v > auditMaxPageSize {
			return nil, fmt.Sprintf("limit must be <= %d", auditMaxPageSize)
		}
		f.pageSize = int32(v)
	}

	if allowCursor {
		if raw := pick("cursor"); raw != nil {
			ts, id, err := decodeCursor(*raw)
			if err != nil {
				return nil, "invalid cursor"
			}
			f.cursorTs = ts
			f.cursorID = id
		}
	}

	return f, ""
}

// toSearchParams maps a filter to sqlc's typed params.
func (f *auditFilter) toSearchParams(limit int32) storedb.SearchAuditParams {
	return storedb.SearchAuditParams{
		Limit:        limit,
		Actor:        f.actor,
		Action:       f.action,
		Result:       f.outcome,
		TargetPrefix: f.resource,
		Since:        f.since,
		Until:        f.until,
		CursorTs:     f.cursorTs,
		CursorID:     f.cursorID,
	}
}

// List handles GET /audit.
func (h *AuditReadHandler) List(w http.ResponseWriter, r *http.Request) {
	f, errMsg := parseFilter(r.URL.Query(), true)
	if errMsg != "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_request", errMsg)
		return
	}

	// When source_ip filtering is in play we may need to pull more rows
	// than the page size to account for ones that get rejected. We cap
	// the over-fetch at 4x to avoid pathological scans; the cursor still
	// points at the last *returned* row so the next page resumes
	// correctly.
	overfetch := int32(1)
	if f.srcCIDR != nil {
		overfetch = 4
	}
	rows, err := h.Q.SearchAudit(r.Context(), f.toSearchParams(f.pageSize*overfetch))
	if err != nil {
		h.Logger.Error("audit search", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", "search failed")
		return
	}

	out := make([]AuditEvent, 0, len(rows))
	for _, r := range rows {
		if !matchesSourceIP(r, f.srcCIDR) {
			continue
		}
		out = append(out, rowToEvent(r))
		if int32(len(out)) >= f.pageSize {
			break
		}
	}

	resp := AuditListResponse{Items: out}
	if int32(len(out)) == f.pageSize && len(out) > 0 {
		last := out[len(out)-1]
		resp.NextCursor = encodeCursor(last.Timestamp, last.ID)
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, resp)
}

// Summary handles GET /audit/summary.
func (h *AuditReadHandler) Summary(w http.ResponseWriter, r *http.Request) {
	f, errMsg := parseFilter(r.URL.Query(), false)
	if errMsg != "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_request", errMsg)
		return
	}
	rows, err := h.Q.SummaryAudit(r.Context(), storedb.SummaryAuditParams{
		Since: f.since, Until: f.until,
	})
	if err != nil {
		h.Logger.Error("audit summary", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", "summary failed")
		return
	}
	out := make([]AuditSummaryRow, len(rows))
	for i, r := range rows {
		out[i] = AuditSummaryRow{
			Actor:   r.Actor,
			Action:  r.Action,
			Outcome: r.Result,
			Count:   r.Count,
		}
	}
	middleware.WriteJSON(w, h.Logger, http.StatusOK, out)
}

// flusher wraps an http.ResponseWriter for periodic flushes during the
// streaming export. If the underlying writer doesn't support Flush we
// just no-op and rely on the kernel buffer flushing on close — but every
// modern net/http ResponseWriter implements http.Flusher.
type flusher struct {
	w http.ResponseWriter
	f http.Flusher
	n int
}

func newFlusher(w http.ResponseWriter) *flusher {
	fl, _ := w.(http.Flusher)
	return &flusher{w: w, f: fl}
}

// flushEvery flushes every n rows.
func (fl *flusher) flushEvery(n int) {
	fl.n++
	if fl.f != nil && fl.n%n == 0 {
		fl.f.Flush()
	}
}

// Export handles GET /audit/export?format=csv|jsonl.
//
// Streams rows directly to the response writer using the same filters as
// List (cursor is ignored — exports always start from "now" and walk
// back). The endpoint:
//
//   - Enforces one concurrent export per actor.
//   - Aborts cleanly when the client disconnects (via r.Context()).
//   - Writes its own audit_log entry recording who exported what.
func (h *AuditReadHandler) Export(w http.ResponseWriter, r *http.Request) {
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "jsonl" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_request", "format must be csv or jsonl")
		return
	}

	f, errMsg := parseFilter(r.URL.Query(), false)
	if errMsg != "" {
		middleware.WriteError(w, http.StatusBadRequest, "bad_request", errMsg)
		return
	}

	// Identify the actor for the per-user concurrency lock and the
	// self-audit row. When auth is disabled (tests) we fall back to a
	// stable sentinel.
	actorKey := "anonymous"
	var actorPtr *string
	if id, ok := auth.IdentityFromContext(r.Context()); ok && id != nil {
		if id.PreferredName != "" {
			actorKey = id.PreferredName
		} else if id.Subject != "" {
			actorKey = id.Subject
		}
		ak := actorKey
		actorPtr = &ak
	}

	if _, busy := h.exportLocks.LoadOrStore(actorKey, struct{}{}); busy {
		middleware.WriteError(w, http.StatusTooManyRequests, "rate_limited",
			"another export is already running for this user")
		return
	}
	defer h.exportLocks.Delete(actorKey)

	// Headers must be set BEFORE the first write. Chunked transfer
	// encoding is implicit when we don't set Content-Length.
	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="audit.csv"`)
	case "jsonl":
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", `attachment; filename="audit.jsonl"`)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	fl := newFlusher(w)

	// Page through SearchAudit until we run out of rows or the client
	// disconnects. Page size of 500 keeps the SQL working set small.
	const pageSize = int32(500)
	cursorTs := f.cursorTs
	cursorID := f.cursorID
	rowCount := 0

	var csvW *csv.Writer
	if format == "csv" {
		csvW = csv.NewWriter(w)
		_ = csvW.Write([]string{
			"id", "timestamp", "actor", "action", "resource",
			"outcome", "request_id", "payload",
		})
	}

	for {
		// Honor client cancellation aggressively.
		if err := r.Context().Err(); err != nil {
			break
		}
		params := f.toSearchParams(pageSize)
		params.CursorTs = cursorTs
		params.CursorID = cursorID
		rows, err := h.Q.SearchAudit(r.Context(), params)
		if err != nil {
			// Headers are already sent — best we can do is log and stop.
			h.Logger.Error("audit export search", "err", err)
			break
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			if r.Context().Err() != nil {
				break
			}
			if !matchesSourceIP(row, f.srcCIDR) {
				cursorTs = row.Ts
				id := row.ID
				cursorID = &id
				continue
			}
			ev := rowToEvent(row)
			if format == "csv" {
				_ = csvW.Write([]string{
					strconv.FormatInt(ev.ID, 10),
					ev.Timestamp.Format(time.RFC3339Nano),
					ev.Actor,
					ev.Action,
					ev.Resource,
					ev.Outcome,
					ev.RequestID,
					string(ev.Payload),
				})
				csvW.Flush()
			} else {
				if err := json.NewEncoder(w).Encode(ev); err != nil {
					h.Logger.Warn("audit export encode", "err", err)
					break
				}
			}
			fl.flushEvery(50)
			rowCount++
			cursorTs = row.Ts
			id := row.ID
			cursorID = &id
		}
		if int32(len(rows)) < pageSize {
			break
		}
	}
	if csvW != nil {
		csvW.Flush()
	}
	if fl.f != nil {
		fl.f.Flush()
	}

	// Self-audit: record the export. Failures are logged but never
	// surface to the client — we already sent 200 and the data.
	payload, _ := json.Marshal(map[string]any{
		"format":    format,
		"row_count": rowCount,
		"filters": map[string]any{
			"actor":     deref(f.actor),
			"action":    deref(f.action),
			"outcome":   deref(f.outcome),
			"resource":  deref(f.resource),
			"since":     tsString(f.since),
			"until":     tsString(f.until),
			"source_ip": cidrString(f.srcCIDR),
		},
	})
	if err := h.Q.InsertAudit(context.Background(), storedb.InsertAuditParams{
		Actor:     actorPtr,
		Action:    "GET /api/v1/audit/export",
		Target:    "/api/v1/audit/export",
		RequestID: middleware.RequestIDOf(r.Context()),
		Payload:   payload,
		Result:    "accepted",
	}); err != nil {
		h.Logger.Error("audit export self-audit failed", "err", err)
	}
}

// deref unwraps an optional string for logging payloads.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// tsString formats a pgtype.Timestamptz for the self-audit payload.
func tsString(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339Nano)
}

// cidrString formats an *net.IPNet for the self-audit payload.
func cidrString(n *net.IPNet) string {
	if n == nil {
		return ""
	}
	return n.String()
}

// errExportBusy is returned by the lock helper when a concurrent export
// is already running for the actor. Currently unused outside Export but
// declared so future helpers can share the sentinel.
var errExportBusy = errors.New("audit: export already in progress for this user")

var _ = errExportBusy
