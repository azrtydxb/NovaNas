//go:build integration

package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestAudit_RecordedOnPOST asserts the audit middleware writes a row to
// audit_log when a state-changing request lands.
func TestAudit_RecordedOnPOST(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ts := startTestServer(t)

	body := `{"name":"audittank","vdevs":[{"type":"mirror","disks":["/dev/A","/dev/B"]}]}`
	resp, err := http.Post(ts.URL+"/api/v1/pools", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	var n int
	row := pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'POST /api/v1/pools'`)
	if err := row.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Errorf("expected at least one audit row for POST /api/v1/pools")
	}
}
