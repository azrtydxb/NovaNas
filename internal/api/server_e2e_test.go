package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/host/disks"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	"github.com/novanas/nova-nas/internal/jobs"
)

// fakeDispatcher captures dispatch calls so we can verify the wiring
// from HTTP request → handler → dispatch payload.
type fakeDispatcher struct {
	calls []jobs.DispatchInput
}

func (f *fakeDispatcher) Dispatch(_ context.Context, in jobs.DispatchInput) (jobs.DispatchOutput, error) {
	f.calls = append(f.calls, in)
	return jobs.DispatchOutput{JobID: uuid.New()}, nil
}

// fakeManager satisfies all read-side handler interfaces by returning
// fixed data. Each method writes its caller's intent so tests can assert
// "the route resolved to the expected manager call."
type fakeManager struct {
	pools     []pool.Pool
	datasets  []dataset.Dataset
	snapshots []snapshot.Snapshot
	disks     []disks.Disk
}

func (f *fakeManager) List(_ context.Context) ([]pool.Pool, error)     { return f.pools, nil }
func (f *fakeManager) Get(_ context.Context, _ string) (*pool.Detail, error) {
	return &pool.Detail{Pool: pool.Pool{Name: "tank"}}, nil
}
func (f *fakeManager) Importable(_ context.Context) ([]pool.ImportablePool, error) { return nil, nil }

type fakeDS struct{ rows []dataset.Dataset }

func (f *fakeDS) List(_ context.Context, _ string) ([]dataset.Dataset, error) { return f.rows, nil }
func (f *fakeDS) Get(_ context.Context, _ string) (*dataset.Detail, error) {
	return &dataset.Detail{Dataset: dataset.Dataset{Name: "tank/x"}}, nil
}

type fakeSnap struct{ rows []snapshot.Snapshot }

func (f *fakeSnap) List(_ context.Context, _ string) ([]snapshot.Snapshot, error) { return f.rows, nil }

type fakeDisk struct{ rows []disks.Disk }

func (f *fakeDisk) List(_ context.Context) ([]disks.Disk, error) { return f.rows, nil }

func newE2EServer(t *testing.T, disp *fakeDispatcher) http.Handler {
	t.Helper()
	srv := New(Deps{
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Disks:      &fakeDisk{rows: []disks.Disk{{Name: "sda"}}},
		Pools:      &fakeManager{pools: []pool.Pool{{Name: "tank"}}},
		Datasets:   &fakeDS{rows: []dataset.Dataset{{Name: "tank/x"}}},
		Snapshots:  &fakeSnap{rows: []snapshot.Snapshot{{Name: "tank/x@s1"}}},
		Dispatcher: disp,
		// E2E exercises route wiring; auth is bypassed so no real
		// Keycloak (or mock Verifier) is needed. Production wiring lives
		// in cmd/nova-api/main.go.
		AuthDisabled: true,
	})
	return srv.Handler()
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}

// TestE2E_ReadEndpoints walks the read-side endpoints (no dispatcher
// needed) and verifies they return JSON with the expected shape.
func TestE2E_ReadEndpoints(t *testing.T) {
	h := newE2EServer(t, &fakeDispatcher{})

	cases := []struct {
		name, path string
		want       string // substring expected in body
	}{
		{"healthz", "/healthz", `"status"`},
		{"list pools", "/api/v1/pools", `"name":"tank"`},
		{"get pool", "/api/v1/pools/tank", `"tank"`},
		{"list datasets", "/api/v1/datasets", `"name":"tank/x"`},
		{"list snapshots", "/api/v1/snapshots", `"name":"tank/x@s1"`},
		{"list disks", "/api/v1/disks", `"sda"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := do(t, h, http.MethodGet, c.path, "")
			if rr.Code != 200 {
				t.Fatalf("GET %s: code=%d body=%s", c.path, rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), c.want) {
				t.Errorf("GET %s body missing %q: %s", c.path, c.want, rr.Body.String())
			}
		})
	}
}

// TestE2E_WriteDispatchesJobs covers a write path per resource and
// asserts the dispatcher saw the right Kind + Target.
func TestE2E_WriteDispatchesJobs(t *testing.T) {
	disp := &fakeDispatcher{}
	h := newE2EServer(t, disp)

	cases := []struct {
		name, method, path, body string
		wantKind                 jobs.Kind
		wantTarget               string
	}{
		{
			name:       "create pool",
			method:     http.MethodPost,
			path:       "/api/v1/pools",
			body:       `{"name":"tank","vdevs":[{"type":"mirror","disks":["/dev/A","/dev/B"]}]}`,
			wantKind:   jobs.KindPoolCreate,
			wantTarget: "tank",
		},
		{
			name:       "destroy pool",
			method:     http.MethodDelete,
			path:       "/api/v1/pools/tank",
			body:       "",
			wantKind:   jobs.KindPoolDestroy,
			wantTarget: "tank",
		},
		{
			name:       "scrub pool",
			method:     http.MethodPost,
			path:       "/api/v1/pools/tank/scrub?action=start",
			body:       "",
			wantKind:   jobs.KindPoolScrub,
			wantTarget: "tank",
		},
		{
			name:       "trim pool",
			method:     http.MethodPost,
			path:       "/api/v1/pools/tank/trim?action=start",
			body:       "",
			wantKind:   jobs.KindPoolTrim,
			wantTarget: "tank",
		},
		{
			name:       "checkpoint pool",
			method:     http.MethodPost,
			path:       "/api/v1/pools/tank/checkpoint",
			body:       "",
			wantKind:   jobs.KindPoolCheckpoint,
			wantTarget: "tank",
		},
		{
			name:       "create dataset",
			method:     http.MethodPost,
			path:       "/api/v1/datasets",
			body:       `{"parent":"tank","name":"x","type":"filesystem"}`,
			wantKind:   jobs.KindDatasetCreate,
			wantTarget: "tank/x",
		},
		{
			name:       "rename dataset",
			method:     http.MethodPost,
			path:       "/api/v1/datasets/tank%2Fx/rename",
			body:       `{"newName":"tank/y"}`,
			wantKind:   jobs.KindDatasetRename,
			wantTarget: "tank/x", // Target is the source name (audit convention)
		},
		{
			name:       "clone dataset",
			method:     http.MethodPost,
			path:       "/api/v1/datasets/tank%2Fx%40s1/clone",
			body:       `{"target":"tank/clone"}`,
			wantKind:   jobs.KindDatasetClone,
			wantTarget: "tank/clone",
		},
		{
			name:       "snapshot create",
			method:     http.MethodPost,
			path:       "/api/v1/snapshots",
			body:       `{"dataset":"tank/x","name":"s2"}`,
			wantKind:   jobs.KindSnapshotCreate,
			wantTarget: "tank/x@s2",
		},
		{
			name:       "snapshot hold",
			method:     http.MethodPost,
			path:       "/api/v1/snapshots/tank%2Fx%40s1/hold",
			body:       `{"tag":"keep"}`,
			wantKind:   jobs.KindSnapshotHold,
			wantTarget: "tank/x@s1",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := len(disp.calls)
			rr := do(t, h, c.method, c.path, c.body)
			if rr.Code != http.StatusAccepted {
				t.Fatalf("%s %s: code=%d body=%s", c.method, c.path, rr.Code, rr.Body.String())
			}
			if len(disp.calls) != before+1 {
				t.Fatalf("expected one dispatch call, got %d new", len(disp.calls)-before)
			}
			got := disp.calls[len(disp.calls)-1]
			if got.Kind != c.wantKind {
				t.Errorf("kind=%q want %q", got.Kind, c.wantKind)
			}
			if got.Target != c.wantTarget {
				t.Errorf("target=%q want %q", got.Target, c.wantTarget)
			}
		})
	}
}

// TestE2E_BadRequestPaths covers the 400 envelope for malformed input
// and 404 for unknown routes.
func TestE2E_BadRequestPaths(t *testing.T) {
	h := newE2EServer(t, &fakeDispatcher{})

	cases := []struct {
		name, method, path, body string
		wantCode                 int
	}{
		{"create pool with reserved name", http.MethodPost, "/api/v1/pools", `{"name":"mirror","vdevs":[{"type":"stripe","disks":["/dev/A"]}]}`, 400},
		{"create dataset with bad name", http.MethodPost, "/api/v1/datasets", `{"parent":"tank","name":"-bad","type":"filesystem"}`, 400},
		{"create pool with empty body", http.MethodPost, "/api/v1/pools", `{}`, 400},
		{"unknown route", http.MethodGet, "/api/v1/nope", "", 404},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := do(t, h, c.method, c.path, c.body)
			if rr.Code != c.wantCode {
				t.Errorf("%s %s: code=%d want %d body=%s",
					c.method, c.path, rr.Code, c.wantCode, rr.Body.String())
			}
			if c.wantCode == 400 {
				// Error envelope must be JSON with a code+message.
				var env struct {
					Error   string `json:"error"`
					Message string `json:"message"`
				}
				if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
					t.Errorf("body not JSON: %v", err)
				}
				if env.Error == "" {
					t.Errorf("error envelope missing 'error' field: %s", rr.Body.String())
				}
			}
		})
	}
}
