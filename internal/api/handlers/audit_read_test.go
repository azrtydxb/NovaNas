package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/novanas/nova-nas/internal/auth"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// fakeAuditReadQ implements AuditReadQ. It returns the rows assigned in
// `rows` from SearchAudit, paged by the cursor. For Export streaming
// tests it can be configured to yield a large lazily-generated stream.
type fakeAuditReadQ struct {
	rows        []storedb.AuditLog
	summary     []storedb.SummaryAuditRow
	inserted    []storedb.InsertAuditParams
	searchCalls int32

	// genRows, when non-zero, makes SearchAudit synthesize rows on
	// demand (used to verify the Export path doesn't buffer everything).
	genRows int
	// flushCalls records how many times the response writer was flushed
	// during Export. The fake doesn't manage this — the real test wires
	// a custom ResponseWriter and counts itself.
}

func (f *fakeAuditReadQ) SearchAudit(_ context.Context, p storedb.SearchAuditParams) ([]storedb.AuditLog, error) {
	atomic.AddInt32(&f.searchCalls, 1)
	if f.genRows > 0 {
		// Yield up to p.Limit rows per call, walking down the synthetic
		// sequence using the cursor so the export loop terminates.
		start := f.genRows
		if p.CursorID != nil {
			start = int(*p.CursorID) - 1
		}
		out := []storedb.AuditLog{}
		for i := 0; i < int(p.Limit) && start > 0; i++ {
			out = append(out, storedb.AuditLog{
				ID:     int64(start),
				Ts:     pgtype.Timestamptz{Time: time.Unix(int64(1_700_000_000+start), 0).UTC(), Valid: true},
				Action: "GET /api/v1/pools",
				Target: "/api/v1/pools",
				Result: "accepted",
			})
			start--
		}
		return out, nil
	}

	// Apply cursor in-memory against the configured slice.
	out := []storedb.AuditLog{}
	for _, r := range f.rows {
		if p.Actor != nil && (r.Actor == nil || *r.Actor != *p.Actor) {
			continue
		}
		if p.Action != nil && r.Action != *p.Action {
			continue
		}
		if p.Result != nil && r.Result != *p.Result {
			continue
		}
		if p.TargetPrefix != nil && !strings.HasPrefix(r.Target, *p.TargetPrefix) {
			continue
		}
		if p.Since.Valid && r.Ts.Time.Before(p.Since.Time) {
			continue
		}
		if p.Until.Valid && !r.Ts.Time.Before(p.Until.Time) {
			continue
		}
		if p.CursorTs.Valid && p.CursorID != nil {
			if r.Ts.Time.After(p.CursorTs.Time) {
				continue
			}
			if r.Ts.Time.Equal(p.CursorTs.Time) && r.ID >= *p.CursorID {
				continue
			}
		}
		out = append(out, r)
		if int32(len(out)) >= p.Limit {
			break
		}
	}
	return out, nil
}

func (f *fakeAuditReadQ) SummaryAudit(_ context.Context, _ storedb.SummaryAuditParams) ([]storedb.SummaryAuditRow, error) {
	return f.summary, nil
}

func (f *fakeAuditReadQ) InsertAudit(_ context.Context, p storedb.InsertAuditParams) error {
	f.inserted = append(f.inserted, p)
	return nil
}

func mkRow(id int64, ts time.Time, actor, action, target, result string, payload string) storedb.AuditLog {
	var ap *string
	if actor != "" {
		a := actor
		ap = &a
	}
	row := storedb.AuditLog{
		ID:     id,
		Ts:     pgtype.Timestamptz{Time: ts.UTC(), Valid: true},
		Actor:  ap,
		Action: action,
		Target: target,
		Result: result,
	}
	if payload != "" {
		row.Payload = []byte(payload)
	}
	return row
}

func newAuditTestHandler(q AuditReadQ) *AuditReadHandler {
	return &AuditReadHandler{Logger: newDiscardLogger(), Q: q}
}

func TestAuditList_FiltersAndPagination(t *testing.T) {
	base := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	rows := []storedb.AuditLog{
		mkRow(5, base.Add(5*time.Minute), "alice", "POST /api/v1/pools", "/api/v1/pools", "accepted", ""),
		mkRow(4, base.Add(4*time.Minute), "bob", "POST /api/v1/datasets", "/api/v1/datasets/tank/x", "accepted", ""),
		mkRow(3, base.Add(3*time.Minute), "alice", "DELETE /api/v1/datasets", "/api/v1/datasets/tank/x", "rejected", ""),
		mkRow(2, base.Add(2*time.Minute), "bob", "POST /api/v1/snapshots", "/api/v1/snapshots", "accepted", ""),
		mkRow(1, base.Add(1*time.Minute), "alice", "POST /api/v1/pools", "/api/v1/pools/tank/scrub", "accepted", ""),
	}
	q := &fakeAuditReadQ{rows: rows}
	h := newAuditTestHandler(q)

	// Filter by actor=alice
	req := httptest.NewRequest(http.MethodGet, "/audit?actor=alice", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp AuditListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 3 {
		t.Errorf("actor filter returned %d (want 3)", len(resp.Items))
	}
	for _, ev := range resp.Items {
		if ev.Actor != "alice" {
			t.Errorf("expected alice, got %q", ev.Actor)
		}
	}

	// Filter by resource prefix
	req = httptest.NewRequest(http.MethodGet, "/audit?resource=/api/v1/datasets", nil)
	rr = httptest.NewRecorder()
	h.List(rr, req)
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Errorf("resource prefix returned %d (want 2)", len(resp.Items))
	}

	// Filter by action
	req = httptest.NewRequest(http.MethodGet, "/audit?action=POST+%2Fapi%2Fv1%2Fpools", nil)
	rr = httptest.NewRecorder()
	h.List(rr, req)
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Errorf("action filter returned %d (want 2)", len(resp.Items))
	}

	// Filter by outcome
	req = httptest.NewRequest(http.MethodGet, "/audit?outcome=rejected", nil)
	rr = httptest.NewRecorder()
	h.List(rr, req)
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Items) != 1 || resp.Items[0].ID != 3 {
		t.Errorf("outcome filter wrong: %+v", resp.Items)
	}

	// Time-window filter
	since := base.Add(2*time.Minute + 30*time.Second).Format(time.RFC3339Nano)
	req = httptest.NewRequest(http.MethodGet, "/audit?since="+since, nil)
	rr = httptest.NewRecorder()
	h.List(rr, req)
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Items) != 3 {
		t.Errorf("since filter returned %d (want 3)", len(resp.Items))
	}

	// Pagination: limit=2 then follow cursor
	req = httptest.NewRequest(http.MethodGet, "/audit?limit=2", nil)
	rr = httptest.NewRecorder()
	h.List(rr, req)
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Items) != 2 || resp.NextCursor == "" {
		t.Fatalf("first page wrong: %+v", resp)
	}
	first := []int64{resp.Items[0].ID, resp.Items[1].ID}

	req = httptest.NewRequest(http.MethodGet, "/audit?limit=2&cursor="+resp.NextCursor, nil)
	rr = httptest.NewRecorder()
	h.List(rr, req)
	var resp2 AuditListResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp2)
	if len(resp2.Items) != 2 {
		t.Fatalf("second page wrong: %+v", resp2)
	}
	for _, id := range first {
		for _, ev := range resp2.Items {
			if ev.ID == id {
				t.Errorf("page 2 returned page-1 row id=%d", id)
			}
		}
	}
}

func TestAuditList_BadRequest(t *testing.T) {
	cases := []string{
		"/audit?since=garbage",
		"/audit?outcome=mystery",
		"/audit?limit=0",
		"/audit?limit=99999",
		"/audit?source_ip=not.a.cidr",
		"/audit?cursor=!!notbase64!!",
	}
	h := newAuditTestHandler(&fakeAuditReadQ{})
	for _, u := range cases {
		req := httptest.NewRequest(http.MethodGet, u, nil)
		rr := httptest.NewRecorder()
		h.List(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s -> %d (want 400) body=%s", u, rr.Code, rr.Body.String())
		}
	}
}

func TestAuditList_SourceIPCIDR(t *testing.T) {
	base := time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)
	rows := []storedb.AuditLog{
		mkRow(3, base.Add(3*time.Minute), "alice", "POST /pools", "/pools", "accepted", `{"source_ip":"10.0.0.5"}`),
		mkRow(2, base.Add(2*time.Minute), "bob", "POST /pools", "/pools", "accepted", `{"source_ip":"192.168.1.2"}`),
		mkRow(1, base.Add(1*time.Minute), "carol", "POST /pools", "/pools", "accepted", `{}`),
	}
	h := newAuditTestHandler(&fakeAuditReadQ{rows: rows})

	req := httptest.NewRequest(http.MethodGet, "/audit?source_ip=10.0.0.0%2F8", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	var resp AuditListResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Items) != 1 || resp.Items[0].Actor != "alice" {
		t.Errorf("CIDR filter wrong: %+v", resp.Items)
	}
}

func TestAuditSummary(t *testing.T) {
	q := &fakeAuditReadQ{summary: []storedb.SummaryAuditRow{
		{Actor: "alice", Action: "POST /pools", Result: "accepted", Count: 7},
		{Actor: "bob", Action: "DELETE /pools", Result: "rejected", Count: 1},
	}}
	h := newAuditTestHandler(q)
	req := httptest.NewRequest(http.MethodGet, "/audit/summary", nil)
	rr := httptest.NewRecorder()
	h.Summary(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []AuditSummaryRow
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if len(got) != 2 || got[0].Count != 7 || got[0].Outcome != "accepted" {
		t.Errorf("summary mapping wrong: %+v", got)
	}
}

// flushCountingWriter wraps an httptest.ResponseRecorder and counts
// Flush() calls. Used to assert that the streaming export periodically
// flushes instead of buffering everything in memory.
type flushCountingWriter struct {
	*httptest.ResponseRecorder
	flushes int
}

func (f *flushCountingWriter) Flush() { f.flushes++ }

func TestAuditExport_StreamsAndFlushes_CSV(t *testing.T) {
	q := &fakeAuditReadQ{genRows: 1500}
	h := newAuditTestHandler(q)
	req := httptest.NewRequest(http.MethodGet, "/audit/export?format=csv", nil)
	rec := httptest.NewRecorder()
	w := &flushCountingWriter{ResponseRecorder: rec}
	h.Export(w, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/csv") {
		t.Errorf("content-type=%q", got)
	}
	if w.flushes < 5 {
		// 1500 rows / 50 per flush ~= 30 flushes. Be lenient.
		t.Errorf("expected >=5 periodic flushes, got %d", w.flushes)
	}
	// Header line + 1500 data lines.
	r := csv.NewReader(rec.Body)
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1501 {
		t.Errorf("csv rows=%d (want 1501)", len(rows))
	}
	// SearchAudit was called multiple times (paginating through 500 at a
	// time) — confirms we didn't buffer everything in one query.
	if atomic.LoadInt32(&q.searchCalls) < 3 {
		t.Errorf("expected paginated SearchAudit calls, got %d", q.searchCalls)
	}
	// Self-audit row was inserted.
	if len(q.inserted) != 1 {
		t.Fatalf("expected 1 self-audit row, got %d", len(q.inserted))
	}
	if q.inserted[0].Action != "GET /api/v1/audit/export" {
		t.Errorf("self-audit action wrong: %q", q.inserted[0].Action)
	}
}

func TestAuditExport_JSONL(t *testing.T) {
	q := &fakeAuditReadQ{genRows: 5}
	h := newAuditTestHandler(q)
	req := httptest.NewRequest(http.MethodGet, "/audit/export?format=jsonl", nil)
	rec := httptest.NewRecorder()
	w := &flushCountingWriter{ResponseRecorder: rec}
	h.Export(w, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/x-ndjson" {
		t.Errorf("content-type=%q", got)
	}
	lines := strings.Split(strings.TrimRight(rec.Body.String(), "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("jsonl lines=%d (want 5)", len(lines))
	}
	for _, ln := range lines {
		var ev AuditEvent
		if err := json.Unmarshal([]byte(ln), &ev); err != nil {
			t.Errorf("invalid jsonl line %q: %v", ln, err)
		}
	}
}

func TestAuditExport_BadFormat(t *testing.T) {
	h := newAuditTestHandler(&fakeAuditReadQ{})
	req := httptest.NewRequest(http.MethodGet, "/audit/export?format=xml", nil)
	rr := httptest.NewRecorder()
	h.Export(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

// TestAuditExport_OnePerUser asserts that a second concurrent export by
// the same actor returns 429 while the first is still in flight.
func TestAuditExport_OnePerUser(t *testing.T) {
	h := newAuditTestHandler(&fakeAuditReadQ{})
	// Pre-populate the lock as if an export was in flight for "alice".
	id := &auth.Identity{PreferredName: "alice"}
	h.exportLocks.Store("alice", struct{}{})
	req := httptest.NewRequest(http.MethodGet, "/audit/export?format=csv", nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), id))
	rr := httptest.NewRecorder()
	h.Export(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr.Code)
	}
}

func TestAuditExport_AbortOnDisconnect(t *testing.T) {
	q := &fakeAuditReadQ{genRows: 100_000}
	h := newAuditTestHandler(q)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	req := httptest.NewRequest(http.MethodGet, "/audit/export?format=csv", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	w := &flushCountingWriter{ResponseRecorder: rec}
	h.Export(w, req)
	// We don't assert status here (headers were written) — just that we
	// didn't iterate the whole 100k rows.
	if atomic.LoadInt32(&q.searchCalls) > 2 {
		t.Errorf("export kept fetching after ctx cancel: %d calls", q.searchCalls)
	}
}

// Viewers must NOT be able to reach /audit/* — granting reads of the
// audit log to a read-only role leaks who-looked-at-what reconnaissance
// signal. This test verifies the DefaultRoleMap reflects that policy.
func TestAuditPermission_ViewerForbidden(t *testing.T) {
	viewer := &auth.Identity{Roles: []string{"nova-viewer"}}
	if auth.IdentityHasPermission(auth.DefaultRoleMap, viewer, auth.PermAuditRead) {
		t.Errorf("viewer must not have PermAuditRead")
	}
	op := &auth.Identity{Roles: []string{"nova-operator"}}
	if !auth.IdentityHasPermission(auth.DefaultRoleMap, op, auth.PermAuditRead) {
		t.Errorf("operator must have PermAuditRead")
	}
	admin := &auth.Identity{Roles: []string{"nova-admin"}}
	if !auth.IdentityHasPermission(auth.DefaultRoleMap, admin, auth.PermAuditRead) {
		t.Errorf("admin must have PermAuditRead")
	}

	// End-to-end through the RequirePermission middleware: a viewer
	// hitting /audit gets a 403 before the handler runs.
	mw := auth.RequirePermission(auth.DefaultRoleMap, auth.PermAuditRead)
	wrapped := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/audit", nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), viewer))
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("viewer status=%d (want 403)", rr.Code)
	}
}

// Demonstrate the cursor encoder/decoder round-trips so a future
// refactor doesn't silently break paging.
func TestAuditCursorRoundTrip(t *testing.T) {
	ts := time.Now().UTC()
	tok := encodeCursor(ts, 42)
	gotTs, gotID, err := decodeCursor(tok)
	if err != nil {
		t.Fatal(err)
	}
	if gotID == nil || *gotID != 42 {
		t.Errorf("id round-trip: %v", gotID)
	}
	if !gotTs.Time.Equal(ts) {
		t.Errorf("ts round-trip: got=%v want=%v", gotTs.Time, ts)
	}
}

// fmtForLog keeps the SearchAuditParams readable in failure output.
//
//nolint:unused
func fmtForLog(p storedb.SearchAuditParams) string {
	return fmt.Sprintf("limit=%d actor=%v action=%v result=%v target=%v since=%v until=%v cursor_ts=%v cursor_id=%v",
		p.Limit, p.Actor, p.Action, p.Result, p.TargetPrefix, p.Since.Valid, p.Until.Valid, p.CursorTs.Valid, p.CursorID)
}
