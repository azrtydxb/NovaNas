package novanas

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// auditServer wires a tiny chi-free mux that mirrors the three audit
// endpoints. Each test crafts the response shape it needs.
type auditServer struct {
	listResponses []AuditListResponse // returned in order
	listIdx       int32
	summary       []AuditSummaryRow
	exportCSV     string
	exportJSONL   string
	exportStatus  int
}

func (s *auditServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/audit", func(w http.ResponseWriter, r *http.Request) {
		i := atomic.AddInt32(&s.listIdx, 1) - 1
		if int(i) >= len(s.listResponses) {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(AuditListResponse{})
			return
		}
		_ = json.NewEncoder(w).Encode(s.listResponses[i])
	})
	mux.HandleFunc("/api/v1/audit/summary", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(s.summary)
	})
	mux.HandleFunc("/api/v1/audit/export", func(w http.ResponseWriter, r *http.Request) {
		if s.exportStatus != 0 && s.exportStatus != 200 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(s.exportStatus)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "rate_limited",
				"message": "busy",
			})
			return
		}
		switch strings.ToLower(r.URL.Query().Get("format")) {
		case "jsonl":
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(s.exportJSONL))
		default:
			w.Header().Set("Content-Type", "text/csv")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(s.exportCSV))
		}
	})
	return mux
}

func TestSDK_ListAudit_Pagination(t *testing.T) {
	now := time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)
	srv := &auditServer{listResponses: []AuditListResponse{
		{
			Items: []AuditEvent{
				{ID: 5, Timestamp: now, Action: "POST /pools", Resource: "/pools", Outcome: "accepted"},
				{ID: 4, Timestamp: now, Action: "POST /pools", Resource: "/pools", Outcome: "accepted"},
			},
			NextCursor: "abc",
		},
		{
			Items: []AuditEvent{
				{ID: 3, Timestamp: now, Action: "POST /pools", Resource: "/pools", Outcome: "rejected"},
			},
		},
	}}
	hs := httptest.NewServer(srv.handler())
	defer hs.Close()
	c := newTestClient(t, hs)

	var got []int64
	if err := c.IterateAudit(context.Background(), AuditFilter{Actor: "alice"}, 10, func(ev AuditEvent) error {
		got = append(got, ev.ID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 5 || got[2] != 3 {
		t.Errorf("iterate ids=%v", got)
	}
}

func TestSDK_SummarizeAudit(t *testing.T) {
	srv := &auditServer{summary: []AuditSummaryRow{
		{Actor: "alice", Action: "POST /pools", Outcome: "accepted", Count: 3},
	}}
	hs := httptest.NewServer(srv.handler())
	defer hs.Close()
	c := newTestClient(t, hs)
	rows, err := c.SummarizeAudit(context.Background(), time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Count != 3 {
		t.Errorf("rows=%+v", rows)
	}
}

func TestSDK_ExportAudit_CSV_Streams(t *testing.T) {
	// Build a 200-row CSV body and assert the SDK invokes the callback
	// once per row without buffering everything.
	var b strings.Builder
	cw := csv.NewWriter(&b)
	_ = cw.Write([]string{"id", "timestamp", "actor", "action", "resource", "outcome", "request_id", "payload"})
	for i := 1; i <= 200; i++ {
		_ = cw.Write([]string{
			fmt.Sprintf("%d", i),
			time.Unix(int64(1_700_000_000+i), 0).UTC().Format(time.RFC3339Nano),
			"alice", "POST /pools", "/pools", "accepted", "", "",
		})
	}
	cw.Flush()
	srv := &auditServer{exportCSV: b.String()}
	hs := httptest.NewServer(srv.handler())
	defer hs.Close()
	c := newTestClient(t, hs)

	var count int
	if err := c.ExportAudit(context.Background(), AuditExportCSV, AuditFilter{}, func(ev AuditEvent) error {
		count++
		if ev.Action != "POST /pools" {
			return fmt.Errorf("unexpected action %q", ev.Action)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 200 {
		t.Errorf("rows=%d (want 200)", count)
	}
}

func TestSDK_ExportAudit_JSONL(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 5; i++ {
		_, _ = b.WriteString(fmt.Sprintf(`{"id":%d,"timestamp":"2026-04-29T00:00:00Z","action":"GET /x","resource":"/x","outcome":"accepted"}`+"\n", i))
	}
	srv := &auditServer{exportJSONL: b.String()}
	hs := httptest.NewServer(srv.handler())
	defer hs.Close()
	c := newTestClient(t, hs)

	var ids []int64
	if err := c.ExportAudit(context.Background(), AuditExportJSONL, AuditFilter{}, func(ev AuditEvent) error {
		ids = append(ids, ev.ID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 5 || ids[0] != 1 || ids[4] != 5 {
		t.Errorf("ids=%v", ids)
	}
}

func TestSDK_ExportAudit_Busy(t *testing.T) {
	srv := &auditServer{exportStatus: http.StatusTooManyRequests}
	hs := httptest.NewServer(srv.handler())
	defer hs.Close()
	c := newTestClient(t, hs)
	err := c.ExportAudit(context.Background(), AuditExportCSV, AuditFilter{}, func(AuditEvent) error { return nil })
	if !errors.Is(err, ErrAuditExportBusy) {
		t.Errorf("err=%v (want ErrAuditExportBusy)", err)
	}
}

func TestSDK_ExportAudit_CallbackAbort(t *testing.T) {
	var b strings.Builder
	cw := csv.NewWriter(&b)
	_ = cw.Write([]string{"id", "timestamp", "actor", "action", "resource", "outcome", "request_id", "payload"})
	for i := 1; i <= 50; i++ {
		_ = cw.Write([]string{fmt.Sprintf("%d", i), "2026-04-29T00:00:00Z", "alice", "GET /x", "/x", "accepted", "", ""})
	}
	cw.Flush()
	srv := &auditServer{exportCSV: b.String()}
	hs := httptest.NewServer(srv.handler())
	defer hs.Close()
	c := newTestClient(t, hs)
	stop := errors.New("stop")
	var seen int
	err := c.ExportAudit(context.Background(), AuditExportCSV, AuditFilter{}, func(ev AuditEvent) error {
		seen++
		if seen == 3 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) {
		t.Errorf("err=%v (want stop)", err)
	}
	if seen != 3 {
		t.Errorf("seen=%d (want 3)", seen)
	}
}
