package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// --- mocks ---------------------------------------------------------------

type mockSnaps struct {
	mu        sync.Mutex
	created   []string
	destroyed []string
	listed    []snapshot.Snapshot
	createErr error
}

func (m *mockSnaps) Create(_ context.Context, ds, short string, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, ds+"@"+short)
	return m.createErr
}
func (m *mockSnaps) Destroy(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destroyed = append(m.destroyed, name)
	return nil
}
func (m *mockSnaps) List(_ context.Context, _ string) ([]snapshot.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listed, nil
}

type mockDatasets struct{}

func (mockDatasets) Send(_ context.Context, _ string, _ dataset.SendOpts, w io.Writer) error {
	_, err := w.Write([]byte("stream"))
	return err
}

type mockTransport struct {
	called bool
	dst    string
	err    error
	data   []byte
}

func (m *mockTransport) Receive(_ context.Context, _ *storedb.ReplicationTarget, dst string, r io.Reader) error {
	m.called = true
	m.dst = dst
	if m.err != nil {
		return m.err
	}
	b, _ := io.ReadAll(r)
	m.data = b
	return nil
}

type mockQueries struct {
	mu         sync.Mutex
	snapScheds []storedb.SnapshotSchedule
	replScheds []storedb.ReplicationSchedule
	target     storedb.ReplicationTarget
	firedSnap  []pgtype.UUID
	firedRepl  []pgtype.UUID
	resultSnap []string
	resultErr  []*string
}

func (q *mockQueries) ListEnabledSnapshotSchedules(_ context.Context) ([]storedb.SnapshotSchedule, error) {
	return q.snapScheds, nil
}
func (q *mockQueries) ListEnabledReplicationSchedules(_ context.Context) ([]storedb.ReplicationSchedule, error) {
	return q.replScheds, nil
}
func (q *mockQueries) GetReplicationTarget(_ context.Context, _ pgtype.UUID) (storedb.ReplicationTarget, error) {
	return q.target, nil
}
func (q *mockQueries) MarkSnapshotScheduleFired(_ context.Context, arg storedb.MarkSnapshotScheduleFiredParams) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.firedSnap = append(q.firedSnap, arg.ID)
	// Mutate the in-memory schedule so subsequent ticks see updated last_fired_at.
	for i := range q.snapScheds {
		if q.snapScheds[i].ID == arg.ID {
			q.snapScheds[i].LastFiredAt = arg.LastFiredAt
		}
	}
	return nil
}
func (q *mockQueries) MarkReplicationScheduleFired(_ context.Context, arg storedb.MarkReplicationScheduleFiredParams) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.firedRepl = append(q.firedRepl, arg.ID)
	for i := range q.replScheds {
		if q.replScheds[i].ID == arg.ID {
			q.replScheds[i].LastFiredAt = arg.LastFiredAt
		}
	}
	return nil
}
func (q *mockQueries) MarkReplicationScheduleResult(_ context.Context, arg storedb.MarkReplicationScheduleResultParams) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if arg.LastSyncSnapshot != nil {
		q.resultSnap = append(q.resultSnap, *arg.LastSyncSnapshot)
		// reflect last_sync_snapshot
		for i := range q.replScheds {
			if q.replScheds[i].ID == arg.ID {
				q.replScheds[i].LastSyncSnapshot = arg.LastSyncSnapshot
			}
		}
	}
	q.resultErr = append(q.resultErr, arg.LastError)
	return nil
}

// --- helpers -------------------------------------------------------------

func newUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.New(), Valid: true}
}

func mkSnapSched(dataset, prefix, cron string, retentionDaily int) storedb.SnapshotSchedule {
	return storedb.SnapshotSchedule{
		ID:             newUUID(),
		Dataset:        dataset,
		Name:           "test",
		Cron:           cron,
		Recursive:      false,
		SnapshotPrefix: prefix,
		RetentionDaily: int32(retentionDaily),
		Enabled:        true,
	}
}

// --- tests ---------------------------------------------------------------

func TestFireSnapshot_NameFormat(t *testing.T) {
	snaps := &mockSnaps{}
	q := &mockQueries{}
	m := New(slog.Default(), q, snaps, mockDatasets{}, &mockTransport{})

	now := time.Date(2026, 4, 28, 14, 30, 0, 0, time.UTC)
	sched := mkSnapSched("tank/data", "auto", "* * * * *", 0)
	if err := m.fireSnapshot(context.Background(), sched, now); err != nil {
		t.Fatal(err)
	}
	if len(snaps.created) != 1 {
		t.Fatalf("expected 1 create, got %d", len(snaps.created))
	}
	want := "tank/data@auto-2026-04-28-1430"
	if snaps.created[0] != want {
		t.Errorf("got %q, want %q", snaps.created[0], want)
	}
}

func TestTick_FiresWhenDue(t *testing.T) {
	snaps := &mockSnaps{}
	q := &mockQueries{
		snapScheds: []storedb.SnapshotSchedule{
			mkSnapSched("tank/data", "auto", "* * * * *", 0),
		},
	}
	now := time.Date(2026, 4, 28, 14, 30, 0, 0, time.UTC)
	m := New(slog.Default(), q, snaps, mockDatasets{}, &mockTransport{})
	m.Now = func() time.Time { return now }
	m.Loc = time.UTC
	m.tick(context.Background())

	if len(snaps.created) != 1 {
		t.Errorf("expected 1 snapshot creation, got %d", len(snaps.created))
	}
	if len(q.firedSnap) != 1 {
		t.Errorf("expected 1 mark-fired, got %d", len(q.firedSnap))
	}
}

func TestTick_DoesNotFireTwiceInSameMinute(t *testing.T) {
	snaps := &mockSnaps{}
	q := &mockQueries{
		snapScheds: []storedb.SnapshotSchedule{
			mkSnapSched("tank/data", "auto", "*/5 * * * *", 0),
		},
	}
	// Pin the schedule's "minute we care about".
	now := time.Date(2026, 4, 28, 14, 30, 0, 0, time.UTC)
	m := New(slog.Default(), q, snaps, mockDatasets{}, &mockTransport{})
	m.Now = func() time.Time { return now }
	m.Loc = time.UTC

	// First tick fires at 14:30.
	m.tick(context.Background())
	// Second tick 30s later — last_fired_at == 14:30 → no new fire.
	m.Now = func() time.Time { return now.Add(30 * time.Second) }
	m.tick(context.Background())

	if len(snaps.created) != 1 {
		t.Errorf("expected exactly 1 create across two ticks, got %d", len(snaps.created))
	}
}

func TestTick_RetentionPrunes(t *testing.T) {
	loc := time.UTC
	// 5 daily snaps under "auto" prefix; retention=2 → 3 should be pruned.
	var listed []snapshot.Snapshot
	for d := 0; d < 5; d++ {
		short := FormatSnapTime("auto", time.Date(2026, 4, 20+d, 0, 0, 0, 0, loc))
		listed = append(listed, snapshot.Snapshot{
			Name:      "tank/data@" + short,
			Dataset:   "tank/data",
			ShortName: short,
		})
	}
	// Add one foreign snapshot — must not be touched.
	listed = append(listed, snapshot.Snapshot{
		Name:      "tank/data@manual",
		Dataset:   "tank/data",
		ShortName: "manual",
	})
	snaps := &mockSnaps{listed: listed}
	q := &mockQueries{
		snapScheds: []storedb.SnapshotSchedule{
			// Use a cron that won't fire to isolate the prune behavior.
			mkSnapSched("tank/data", "auto", "0 0 1 1 *", 2),
		},
	}
	// Set last_fired_at so the cron predicate is "already fired".
	q.snapScheds[0].LastFiredAt = pgtype.Timestamptz{
		Time:  time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
		Valid: true,
	}
	m := New(slog.Default(), q, snaps, mockDatasets{}, &mockTransport{})
	m.Now = func() time.Time { return time.Date(2026, 4, 28, 12, 0, 0, 0, loc) }
	m.Loc = loc
	m.tick(context.Background())
	if len(snaps.destroyed) != 3 {
		t.Errorf("expected 3 destroyed, got %d (%v)", len(snaps.destroyed), snaps.destroyed)
	}
	for _, d := range snaps.destroyed {
		if d == "tank/data@manual" {
			t.Errorf("foreign snapshot was destroyed")
		}
	}
}

func TestFireReplication_FullSend(t *testing.T) {
	snaps := &mockSnaps{}
	transport := &mockTransport{}
	target := storedb.ReplicationTarget{
		ID:            newUUID(),
		Name:          "remote",
		Host:          "10.0.0.1",
		Port:          22,
		SshUser:       "repl",
		SshKeyPath:    "/etc/nova-nas/repl.key",
		DatasetPrefix: "backup/from-tank",
	}
	repl := storedb.ReplicationSchedule{
		ID:             newUUID(),
		SrcDataset:     "tank/data",
		TargetID:       target.ID,
		Cron:           "* * * * *",
		SnapshotPrefix: "repl",
		Enabled:        true,
	}
	q := &mockQueries{
		replScheds: []storedb.ReplicationSchedule{repl},
		target:     target,
	}
	m := New(slog.Default(), q, snaps, mockDatasets{}, transport)
	m.Now = func() time.Time { return time.Date(2026, 4, 28, 14, 30, 0, 0, time.UTC) }
	m.Loc = time.UTC

	if err := m.fireReplication(context.Background(), repl, m.Now()); err != nil {
		t.Fatal(err)
	}
	if !transport.called {
		t.Fatal("transport not called")
	}
	if transport.dst != "backup/from-tank/data" {
		t.Errorf("dst %q", transport.dst)
	}
	if string(transport.data) != "stream" {
		t.Errorf("data %q", transport.data)
	}
	if len(q.resultSnap) != 1 || q.resultSnap[0] != "tank/data@repl-2026-04-28-1430" {
		t.Errorf("result %v", q.resultSnap)
	}
}

func TestFireReplication_TransportErrorPropagates(t *testing.T) {
	snaps := &mockSnaps{}
	transport := &mockTransport{err: errors.New("ssh boom")}
	target := storedb.ReplicationTarget{
		ID: newUUID(), Host: "x", Port: 22, SshUser: "u",
		SshKeyPath: "/k", DatasetPrefix: "backup/x",
	}
	repl := storedb.ReplicationSchedule{
		ID: newUUID(), SrcDataset: "tank/data", TargetID: target.ID,
		Cron: "* * * * *", SnapshotPrefix: "repl", Enabled: true,
	}
	q := &mockQueries{
		replScheds: []storedb.ReplicationSchedule{repl},
		target:     target,
	}
	m := New(slog.Default(), q, snaps, mockDatasets{}, transport)
	m.Now = func() time.Time { return time.Date(2026, 4, 28, 14, 30, 0, 0, time.UTC) }
	m.Loc = time.UTC
	err := m.fireReplication(context.Background(), repl, m.Now())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRemoteDataset(t *testing.T) {
	cases := []struct {
		prefix, src, want string
	}{
		{"backup/from-tank", "tank/data", "backup/from-tank/data"},
		{"backup/from-tank/", "tank/data", "backup/from-tank/data"},
		{"backup", "tank", "backup/tank"},
		{"backup/from-tank", "tank/projects/alpha", "backup/from-tank/alpha"},
	}
	for _, tc := range cases {
		got := remoteDataset(tc.prefix, tc.src)
		if got != tc.want {
			t.Errorf("remoteDataset(%q, %q) = %q, want %q", tc.prefix, tc.src, got, tc.want)
		}
	}
}
