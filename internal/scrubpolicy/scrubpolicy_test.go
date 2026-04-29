package scrubpolicy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/novanas/nova-nas/internal/jobs"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// fakeQ is an in-memory replacement for storedb.Queries, indexed by id.
type fakeQ struct {
	mu       sync.Mutex
	rows     map[string]*storedb.ScrubPolicy
	byName   map[string]*storedb.ScrubPolicy
	fireCall []storedb.MarkScrubPolicyFiredParams
}

func newFakeQ() *fakeQ {
	return &fakeQ{
		rows:   map[string]*storedb.ScrubPolicy{},
		byName: map[string]*storedb.ScrubPolicy{},
	}
}

func (f *fakeQ) ListEnabledScrubPolicies(_ context.Context) ([]storedb.ScrubPolicy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storedb.ScrubPolicy, 0, len(f.rows))
	for _, r := range f.rows {
		if r.Enabled {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (f *fakeQ) GetScrubPolicyByName(_ context.Context, name string) (storedb.ScrubPolicy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.byName[name]; ok {
		return *r, nil
	}
	return storedb.ScrubPolicy{}, pgx.ErrNoRows
}

func (f *fakeQ) CreateScrubPolicy(_ context.Context, arg storedb.CreateScrubPolicyParams) (storedb.ScrubPolicy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.byName[arg.Name]; exists {
		return storedb.ScrubPolicy{}, errors.New("duplicate name")
	}
	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	row := &storedb.ScrubPolicy{
		ID: id, Name: arg.Name, Pools: arg.Pools, Cron: arg.Cron,
		Priority: arg.Priority, Enabled: arg.Enabled, Builtin: arg.Builtin,
	}
	f.rows[uuid.UUID(id.Bytes).String()] = row
	f.byName[arg.Name] = row
	return *row, nil
}

func (f *fakeQ) MarkScrubPolicyFired(_ context.Context, arg storedb.MarkScrubPolicyFiredParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fireCall = append(f.fireCall, arg)
	if r, ok := f.rows[uuid.UUID(arg.ID.Bytes).String()]; ok {
		r.LastFiredAt = arg.LastFiredAt
		r.LastError = arg.LastError
	}
	return nil
}

// fakePools is the PoolsAPI fake.
type fakePools struct {
	pools      []string
	inProgress map[string]bool
	listErr    error
}

func (p *fakePools) PoolNames(_ context.Context) ([]string, error) {
	if p.listErr != nil {
		return nil, p.listErr
	}
	cp := append([]string(nil), p.pools...)
	return cp, nil
}

func (p *fakePools) IsScrubInProgress(_ context.Context, name string) (bool, error) {
	return p.inProgress[name], nil
}

// fakeDispatcher records calls.
type fakeDispatcher struct {
	mu    sync.Mutex
	calls []jobs.DispatchInput
	err   error
}

func (d *fakeDispatcher) Dispatch(_ context.Context, in jobs.DispatchInput) (jobs.DispatchOutput, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, in)
	if d.err != nil {
		return jobs.DispatchOutput{}, d.err
	}
	return jobs.DispatchOutput{JobID: uuid.New()}, nil
}

// ----------------- tests -----------------

func TestEnsureDefaultPolicy_Creates(t *testing.T) {
	q := newFakeQ()
	m := New(nil, q, &fakePools{}, &fakeDispatcher{})
	row, err := m.EnsureDefaultPolicy(context.Background())
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if row.Name != DefaultPolicyName || row.Cron != DefaultCron {
		t.Errorf("got %+v", row)
	}
	if !row.Builtin || !row.Enabled {
		t.Errorf("expected builtin+enabled")
	}
	// Re-running is idempotent.
	row2, err := m.EnsureDefaultPolicy(context.Background())
	if err != nil {
		t.Fatalf("ensure2: %v", err)
	}
	if row2.Name != row.Name {
		t.Errorf("re-run created different row: %v vs %v", row.Name, row2.Name)
	}
	if len(q.rows) != 1 {
		t.Errorf("expected 1 row after re-run, got %d", len(q.rows))
	}
}

func TestValidateCron(t *testing.T) {
	if err := ValidateCron("0 2 * * 0"); err != nil {
		t.Errorf("good cron rejected: %v", err)
	}
	if err := ValidateCron("not a cron"); err == nil {
		t.Errorf("bad cron accepted")
	}
	if err := ValidateCron("0 2 * *"); err == nil { // 4 fields
		t.Errorf("4-field cron accepted")
	}
}

func TestValidatePriority(t *testing.T) {
	for _, v := range []string{"", "low", "medium", "high"} {
		if err := ValidatePriority(v); err != nil {
			t.Errorf("%q rejected: %v", v, err)
		}
	}
	if err := ValidatePriority("urgent"); err == nil {
		t.Errorf("urgent accepted")
	}
}

func TestExpandPools_Wildcard(t *testing.T) {
	pools := &fakePools{pools: []string{"tank", "scratch"}}
	m := New(nil, newFakeQ(), pools, &fakeDispatcher{})
	got, err := m.expandPools(context.Background(), "*")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("wildcard expand: %v", got)
	}
}

func TestExpandPools_List(t *testing.T) {
	m := New(nil, newFakeQ(), &fakePools{}, &fakeDispatcher{})
	got, err := m.expandPools(context.Background(), "tank, scratch , vault")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != "tank" || got[2] != "vault" {
		t.Errorf("list expand: %v", got)
	}
}

func TestTick_FiresAndDispatches(t *testing.T) {
	q := newFakeQ()
	pools := &fakePools{pools: []string{"tank", "scratch"}}
	disp := &fakeDispatcher{}

	// Manually insert an enabled policy with cron that always matches.
	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	q.rows[uuid.UUID(id.Bytes).String()] = &storedb.ScrubPolicy{
		ID: id, Name: "p1", Pools: "*",
		Cron: "* * * * *", Priority: "high", Enabled: true,
	}
	q.byName["p1"] = q.rows[uuid.UUID(id.Bytes).String()]

	m := New(nil, q, pools, disp)
	m.MinFireGap = 0
	m.Now = func() time.Time { return time.Date(2024, 1, 7, 2, 0, 0, 0, time.UTC) } // a Sunday at 02:00

	m.Tick(context.Background())
	if len(disp.calls) != 2 {
		t.Fatalf("want 2 dispatches, got %d", len(disp.calls))
	}
	for _, c := range disp.calls {
		if c.Kind != jobs.KindPoolScrub {
			t.Errorf("kind=%s want pool.scrub", c.Kind)
		}
		if c.UniqueKey == "" || c.Target == "" {
			t.Errorf("missing key/target: %+v", c)
		}
	}
	if len(q.fireCall) != 1 {
		t.Errorf("want 1 mark-fired call, got %d", len(q.fireCall))
	}
}

func TestTick_SkipsPoolWithScrubInProgress(t *testing.T) {
	q := newFakeQ()
	pools := &fakePools{
		pools:      []string{"tank", "scratch"},
		inProgress: map[string]bool{"tank": true},
	}
	disp := &fakeDispatcher{}

	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	q.rows[uuid.UUID(id.Bytes).String()] = &storedb.ScrubPolicy{
		ID: id, Name: "p1", Pools: "*",
		Cron: "* * * * *", Priority: "medium", Enabled: true,
	}
	q.byName["p1"] = q.rows[uuid.UUID(id.Bytes).String()]

	m := New(nil, q, pools, disp)
	m.MinFireGap = 0
	m.Now = func() time.Time { return time.Date(2024, 1, 7, 2, 0, 0, 0, time.UTC) }

	m.Tick(context.Background())
	if len(disp.calls) != 1 || disp.calls[0].Target != "scratch" {
		t.Errorf("expected only scratch dispatched, got %v", disp.calls)
	}
}

func TestTick_RespectsMinFireGap(t *testing.T) {
	q := newFakeQ()
	pools := &fakePools{pools: []string{"tank"}}
	disp := &fakeDispatcher{}

	now := time.Date(2024, 1, 7, 2, 0, 0, 0, time.UTC)
	// Last fired 2 hours ago — gap is 7 days by default, should NOT fire.
	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	q.rows[uuid.UUID(id.Bytes).String()] = &storedb.ScrubPolicy{
		ID: id, Name: "p1", Pools: "*",
		Cron: "* * * * *", Priority: "medium", Enabled: true,
		LastFiredAt: pgtype.Timestamptz{Time: now.Add(-2 * time.Hour), Valid: true},
	}
	q.byName["p1"] = q.rows[uuid.UUID(id.Bytes).String()]

	m := New(nil, q, pools, disp)
	m.Now = func() time.Time { return now }

	m.Tick(context.Background())
	if len(disp.calls) != 0 {
		t.Errorf("MinFireGap not enforced: %v", disp.calls)
	}
}

func TestTick_BadCronLogsAndContinues(t *testing.T) {
	q := newFakeQ()
	pools := &fakePools{pools: []string{"tank"}}
	disp := &fakeDispatcher{}

	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	q.rows[uuid.UUID(id.Bytes).String()] = &storedb.ScrubPolicy{
		ID: id, Name: "p1", Pools: "*",
		Cron: "not a cron", Priority: "medium", Enabled: true,
	}
	q.byName["p1"] = q.rows[uuid.UUID(id.Bytes).String()]

	m := New(nil, q, pools, disp)
	m.Tick(context.Background()) // must not panic
	if len(disp.calls) != 0 {
		t.Errorf("dispatched despite bad cron: %v", disp.calls)
	}
}
