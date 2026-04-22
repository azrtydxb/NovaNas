package apismoke

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestAPISmoke exercises every listed route against a running NovaNas.
// Skipped unless E2E_RUN=1 is set — the default CI invocation provides the
// env.
func TestAPISmoke(t *testing.T) {
	if os.Getenv("E2E_RUN") != "1" {
		t.Skip("set E2E_RUN=1 with a reachable NOVANAS_BASE_URL to run API smokes")
	}
	cfg := ConfigFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	results := Run(ctx, cfg)
	if len(results) != len(Routes) {
		t.Fatalf("expected %d results, got %d", len(Routes), len(results))
	}
	for _, r := range results {
		if !r.Ok() {
			t.Errorf("route %s %s: status=%d err=%v (want one of %v)",
				r.Endpoint.Method, r.Endpoint.Path, r.Status, r.Err, r.Endpoint.StatusCodes)
		}
	}
}

// TestRoutesCoverage is a static sanity test — it verifies the Routes table
// covers the documented 14 route modules even without a running backend.
func TestRoutesCoverage(t *testing.T) {
	modules := map[string]bool{
		"pools": false, "datasets": false, "buckets": false, "shares": false,
		"disks": false, "snapshots": false, "replication": false, "apps": false,
		"vms": false, "system": false, "identity": false,
		"health": false, "version": false, "ready": false,
	}
	for _, r := range Routes {
		for k := range modules {
			if contains(r.Path, k) {
				modules[k] = true
			}
		}
	}
	for k, seen := range modules {
		if !seen {
			t.Errorf("no smoke route covers module %q", k)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
