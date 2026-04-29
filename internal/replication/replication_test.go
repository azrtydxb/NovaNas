package replication

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeBackend is a Backend used by manager tests. It records the last
// Execute invocation and returns whatever the test configures.
type fakeBackend struct {
	kind     BackendKind
	validate func(Job) error
	exec     func(Job) (RunResult, error)
}

func (f *fakeBackend) Kind() BackendKind                          { return f.kind }
func (f *fakeBackend) Validate(_ context.Context, j Job) error    { return f.validate(j) }
func (f *fakeBackend) Execute(_ context.Context, in ExecuteContext) (RunResult, error) {
	return f.exec(in.Job)
}

func newTestManager(t *testing.T, b Backend) *Manager {
	t.Helper()
	return NewManager(NewMemStore(), NewMemLocker(), []Backend{b}, ManagerOptions{
		Now: func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) },
	})
}

func TestJobValidate(t *testing.T) {
	cases := []struct {
		name string
		j    Job
		err  bool
	}{
		{"empty", Job{}, true},
		{"missing backend", Job{Name: "x"}, true},
		{"missing direction", Job{Name: "x", Backend: BackendZFS}, true},
		{"ok", Job{Name: "x", Backend: BackendZFS, Direction: DirectionPush}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.j.Validate()
			if tc.err && err == nil {
				t.Fatalf("expected error")
			}
			if !tc.err && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestManagerCreateAndGet(t *testing.T) {
	be := &fakeBackend{
		kind:     BackendZFS,
		validate: func(Job) error { return nil },
		exec:     func(Job) (RunResult, error) { return RunResult{}, nil },
	}
	m := newTestManager(t, be)
	ctx := context.Background()

	created, err := m.Create(ctx, Job{
		Name: "nightly", Backend: BackendZFS, Direction: DirectionPush,
		Source:      Source{Dataset: "tank/data"},
		Destination: Destination{Dataset: "backup/data", Host: "backup.local"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == uuid.Nil {
		t.Fatal("ID not assigned")
	}
	got, err := m.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "nightly" {
		t.Fatalf("Name=%q want nightly", got.Name)
	}
}

func TestManagerCreateRejectsUnknownBackend(t *testing.T) {
	be := &fakeBackend{kind: BackendZFS, validate: func(Job) error { return nil }}
	m := newTestManager(t, be)
	if _, err := m.Create(context.Background(), Job{
		Name: "x", Backend: BackendS3, Direction: DirectionPush,
	}); err == nil {
		t.Fatal("expected unknown-backend error")
	}
}

func TestManagerRunSuccessUpdatesLastSnapshot(t *testing.T) {
	be := &fakeBackend{
		kind:     BackendZFS,
		validate: func(Job) error { return nil },
		exec:     func(Job) (RunResult, error) { return RunResult{BytesTransferred: 42, Snapshot: "tank/data@repl-1"}, nil },
	}
	m := newTestManager(t, be)
	ctx := context.Background()
	job, _ := m.Create(ctx, Job{
		Name: "n", Backend: BackendZFS, Direction: DirectionPush,
	})
	run, err := m.Run(ctx, job.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.Outcome != RunSucceeded {
		t.Fatalf("Outcome=%v", run.Outcome)
	}
	if run.BytesTransferred != 42 {
		t.Fatalf("Bytes=%d", run.BytesTransferred)
	}
	got, _ := m.Get(ctx, job.ID)
	if got.LastSnapshot != "tank/data@repl-1" {
		t.Fatalf("LastSnapshot=%q", got.LastSnapshot)
	}
}

func TestManagerRunFailureRecordsError(t *testing.T) {
	want := errors.New("send failed")
	be := &fakeBackend{
		kind:     BackendZFS,
		validate: func(Job) error { return nil },
		exec:     func(Job) (RunResult, error) { return RunResult{}, want },
	}
	m := newTestManager(t, be)
	job, _ := m.Create(context.Background(), Job{Name: "n", Backend: BackendZFS, Direction: DirectionPush})
	run, err := m.Run(context.Background(), job.ID)
	if !errors.Is(err, want) {
		t.Fatalf("err=%v want %v", err, want)
	}
	if run.Outcome != RunFailed {
		t.Fatalf("Outcome=%v want failed", run.Outcome)
	}
	if run.Error == "" {
		t.Fatal("Error not set")
	}
}

func TestManagerRunLocked(t *testing.T) {
	be := &fakeBackend{
		kind:     BackendZFS,
		validate: func(Job) error { return nil },
		exec:     func(Job) (RunResult, error) { return RunResult{}, nil },
	}
	store := NewMemStore()
	lk := NewMemLocker()
	m := NewManager(store, lk, []Backend{be}, ManagerOptions{})
	job, _ := m.Create(context.Background(), Job{Name: "n", Backend: BackendZFS, Direction: DirectionPush})
	// Pre-acquire the lock to simulate an in-flight run.
	locked, _, err := lk.TryLock(context.Background(), job.ID, time.Hour)
	if err != nil || !locked {
		t.Fatalf("setup lock: locked=%v err=%v", locked, err)
	}
	if _, err := m.Run(context.Background(), job.ID); !errors.Is(err, ErrLocked) {
		t.Fatalf("err=%v want ErrLocked", err)
	}
}

func TestRetentionEmptyPolicyKeepsAll(t *testing.T) {
	recs := []RunRecord{{ID: "a", Time: time.Now()}, {ID: "b", Time: time.Now().Add(-time.Hour)}}
	keep, drop := RetentionApply(recs, RetentionPolicy{})
	if len(keep) != 2 || len(drop) != 0 {
		t.Fatalf("keep=%d drop=%d", len(keep), len(drop))
	}
}

func TestRetentionKeepLastN(t *testing.T) {
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	recs := []RunRecord{
		{ID: "1", Time: now},
		{ID: "2", Time: now.Add(-24 * time.Hour)},
		{ID: "3", Time: now.Add(-48 * time.Hour)},
		{ID: "4", Time: now.Add(-72 * time.Hour)},
	}
	keep, drop := RetentionApply(recs, RetentionPolicy{KeepLastN: 2})
	if len(keep) != 2 {
		t.Fatalf("keep=%d want 2", len(keep))
	}
	if len(drop) != 2 {
		t.Fatalf("drop=%d want 2", len(drop))
	}
	// Two newest must survive.
	gotIDs := map[string]bool{}
	for _, r := range keep {
		gotIDs[r.ID] = true
	}
	if !gotIDs["1"] || !gotIDs["2"] {
		t.Fatalf("kept wrong ids: %v", gotIDs)
	}
}

func TestRetentionDailyBucket(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 1, d, 12, 0, 0, 0, time.UTC) }
	recs := []RunRecord{
		{ID: "d1a", Time: day(1).Add(time.Hour)},
		{ID: "d1b", Time: day(1).Add(2 * time.Hour)},
		{ID: "d2", Time: day(2)},
		{ID: "d3", Time: day(3)},
	}
	keep, _ := RetentionApply(recs, RetentionPolicy{KeepDaily: 2})
	// Two distinct days kept (d3 + d2). Both d1 entries dropped.
	if len(keep) != 2 {
		t.Fatalf("keep=%d", len(keep))
	}
}
