// Package scheduler orchestrates two recurring workloads:
//
//  1. Per-dataset cron-driven snapshots with sanoid-style retention.
//  2. Per-dataset cron-driven replication via zfs send | ssh | zfs receive.
//
// The Manager runs a single tick loop. On each tick it lists enabled
// schedules, evaluates "should fire" against each cron expression, and
// invokes fire methods. Retention pruning runs every tick regardless of
// cron — it's cheap (list + name comparison + destroy) and keeping it
// independent of the cron schedule means deletion catches up even if a
// schedule was disabled then re-enabled.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// SnapshotsAPI is the subset of *snapshot.Manager that the scheduler uses.
// It's an interface to make the manager mockable in unit tests.
type SnapshotsAPI interface {
	Create(ctx context.Context, dataset, short string, recursive bool) error
	Destroy(ctx context.Context, name string) error
	List(ctx context.Context, root string) ([]snapshot.Snapshot, error)
}

// DatasetsAPI is the subset of *dataset.Manager that the scheduler uses
// (for replication: Send).
type DatasetsAPI interface {
	Send(ctx context.Context, snapshot string, opts dataset.SendOpts, w io.Writer) error
}

// Transport is the side that ships a zfs send stream to a remote and
// runs zfs receive there. The default implementation shells out to ssh
// (see ssh.go); tests provide a fake that buffers/discards.
type Transport interface {
	// Run the receive side: open ssh, exec "zfs receive ...", and copy
	// from r (the local zfs send stdout) to ssh stdin. Block until the
	// remote command exits or ctx is cancelled.
	Receive(ctx context.Context, target *storedb.ReplicationTarget, dst string, r io.Reader) error
}

// QueriesAPI is the subset of *storedb.Queries the scheduler uses.
// Defining it as an interface lets us test the tick loop without a real
// database.
type QueriesAPI interface {
	ListEnabledSnapshotSchedules(ctx context.Context) ([]storedb.SnapshotSchedule, error)
	ListEnabledReplicationSchedules(ctx context.Context) ([]storedb.ReplicationSchedule, error)
	GetReplicationTarget(ctx context.Context, id pgtype.UUID) (storedb.ReplicationTarget, error)
	MarkSnapshotScheduleFired(ctx context.Context, arg storedb.MarkSnapshotScheduleFiredParams) error
	MarkReplicationScheduleFired(ctx context.Context, arg storedb.MarkReplicationScheduleFiredParams) error
	MarkReplicationScheduleResult(ctx context.Context, arg storedb.MarkReplicationScheduleResultParams) error
}

// Manager is the scheduler. Construct via New; call Run with a context
// that's cancelled on shutdown.
type Manager struct {
	Logger       *slog.Logger
	Queries      QueriesAPI
	Snapshots    SnapshotsAPI
	Datasets     DatasetsAPI
	Transport    Transport
	Now          func() time.Time
	TickInterval time.Duration
	Loc          *time.Location
}

// New builds a Manager with sensible defaults. Required dependencies must
// be passed via fields after construction (or via this constructor's
// arguments). The signature accepts the "core" deps; everything else is
// settable on the returned struct.
func New(logger *slog.Logger, queries QueriesAPI, snaps SnapshotsAPI, ds DatasetsAPI, transport Transport) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		Logger:       logger,
		Queries:      queries,
		Snapshots:    snaps,
		Datasets:     ds,
		Transport:    transport,
		Now:          time.Now,
		TickInterval: 60 * time.Second,
		Loc:          time.Local,
	}
}

// Run blocks until ctx is done. Returns ctx.Err() on shutdown, never a
// scheduling error — per-schedule errors are logged and never abort the
// loop.
func (m *Manager) Run(ctx context.Context) error {
	if m.TickInterval <= 0 {
		m.TickInterval = 60 * time.Second
	}
	if m.Loc == nil {
		m.Loc = time.Local
	}
	if m.Now == nil {
		m.Now = time.Now
	}
	tk := time.NewTicker(m.TickInterval)
	defer tk.Stop()

	// Fire one tick immediately on start so first-run latency is bounded
	// by the initial work, not by TickInterval.
	m.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tk.C:
			m.tick(ctx)
		}
	}
}

// tick runs one full pass: snapshot schedules, replication schedules,
// and retention pruning across all snapshot schedules.
func (m *Manager) tick(ctx context.Context) {
	now := m.Now().In(m.Loc)
	if err := m.runSnapshotSchedules(ctx, now); err != nil {
		m.Logger.Error("scheduler: snapshot pass failed", "err", err)
	}
	if err := m.runReplicationSchedules(ctx, now); err != nil {
		m.Logger.Error("scheduler: replication pass failed", "err", err)
	}
}

// runSnapshotSchedules lists enabled schedules, fires due ones, and
// prunes per retention.
func (m *Manager) runSnapshotSchedules(ctx context.Context, now time.Time) error {
	scheds, err := m.Queries.ListEnabledSnapshotSchedules(ctx)
	if err != nil {
		return fmt.Errorf("list snapshot schedules: %w", err)
	}
	for _, s := range scheds {
		expr, err := ParseCron(s.Cron)
		if err != nil {
			m.Logger.Warn("scheduler: bad cron, skipping", "id", uuidString(s.ID), "cron", s.Cron, "err", err)
			continue
		}
		var prev time.Time
		if s.LastFiredAt.Valid {
			prev = s.LastFiredAt.Time
		}
		if expr.ShouldFireBetween(prev, now, m.Loc) {
			if err := m.fireSnapshot(ctx, s, now); err != nil {
				m.Logger.Error("scheduler: fire snapshot failed",
					"id", uuidString(s.ID), "dataset", s.Dataset, "err", err)
				// Still mark fired so we don't busy-loop on the same minute.
			}
			if err := m.Queries.MarkSnapshotScheduleFired(ctx, storedb.MarkSnapshotScheduleFiredParams{
				ID:          s.ID,
				LastFiredAt: pgtype.Timestamptz{Time: now, Valid: true},
			}); err != nil {
				m.Logger.Error("scheduler: mark snapshot fired failed", "id", uuidString(s.ID), "err", err)
			}
		}
		// Retention runs every tick, independent of fire schedule.
		if err := m.pruneSnapshots(ctx, s); err != nil {
			m.Logger.Warn("scheduler: prune failed", "id", uuidString(s.ID), "dataset", s.Dataset, "err", err)
		}
	}
	return nil
}

// fireSnapshot creates the snapshot for one schedule. The full short name
// is "<prefix>-<YYYY-MM-DD-HHMM>".
func (m *Manager) fireSnapshot(ctx context.Context, s storedb.SnapshotSchedule, now time.Time) error {
	short := FormatSnapTime(s.SnapshotPrefix, now)
	return m.Snapshots.Create(ctx, s.Dataset, short, s.Recursive)
}

// pruneSnapshots applies the schedule's retention policy. It only
// considers snapshots whose short-name begins with "<prefix>-"; foreign
// snapshots (manual, other prefixes) are ignored.
func (m *Manager) pruneSnapshots(ctx context.Context, s storedb.SnapshotSchedule) error {
	all, err := m.Snapshots.List(ctx, s.Dataset)
	if err != nil {
		return err
	}
	policy := RetentionPolicy{
		Hourly:  int(s.RetentionHourly),
		Daily:   int(s.RetentionDaily),
		Weekly:  int(s.RetentionWeekly),
		Monthly: int(s.RetentionMonthly),
		Yearly:  int(s.RetentionYearly),
	}
	// Skip pruning entirely if the policy is all-zero — that signals
	// "no retention configured". Without this guard, every schedule
	// snapshot would be eligible for destroy on the first tick.
	if policy.Hourly == 0 && policy.Daily == 0 && policy.Weekly == 0 && policy.Monthly == 0 && policy.Yearly == 0 {
		return nil
	}
	wantPrefix := s.SnapshotPrefix + "-"
	var managed []SnapInfo
	for _, sn := range all {
		// `zfs list -r tank` returns descendant snapshots too; restrict
		// pruning to the schedule's exact dataset.
		if sn.Dataset != s.Dataset {
			continue
		}
		if !strings.HasPrefix(sn.ShortName, wantPrefix) {
			continue
		}
		t, ok := ParsedSnapTime(sn.ShortName, s.SnapshotPrefix, m.Loc)
		if !ok {
			// Has the prefix but not our timestamp format — leave alone.
			continue
		}
		managed = append(managed, SnapInfo{Name: sn.Name, Time: t})
	}
	_, drop := PartitionRetention(managed, policy)
	for _, d := range drop {
		if err := m.Snapshots.Destroy(ctx, d.Name); err != nil {
			m.Logger.Warn("scheduler: destroy snapshot failed", "name", d.Name, "err", err)
			// Continue — one bad snapshot shouldn't block the rest.
		}
	}
	return nil
}

// runReplicationSchedules lists enabled replication schedules and fires
// due ones.
func (m *Manager) runReplicationSchedules(ctx context.Context, now time.Time) error {
	scheds, err := m.Queries.ListEnabledReplicationSchedules(ctx)
	if err != nil {
		return fmt.Errorf("list replication schedules: %w", err)
	}
	for _, s := range scheds {
		expr, err := ParseCron(s.Cron)
		if err != nil {
			m.Logger.Warn("scheduler: bad cron, skipping", "id", uuidString(s.ID), "cron", s.Cron, "err", err)
			continue
		}
		var prev time.Time
		if s.LastFiredAt.Valid {
			prev = s.LastFiredAt.Time
		}
		if !expr.ShouldFireBetween(prev, now, m.Loc) {
			continue
		}
		if err := m.fireReplication(ctx, s, now); err != nil {
			msg := err.Error()
			if mErr := m.Queries.MarkReplicationScheduleResult(ctx, storedb.MarkReplicationScheduleResultParams{
				ID:               s.ID,
				LastSyncSnapshot: s.LastSyncSnapshot,
				LastError:        &msg,
			}); mErr != nil {
				m.Logger.Error("scheduler: mark replication result failed", "id", uuidString(s.ID), "err", mErr)
			}
			m.Logger.Error("scheduler: fire replication failed",
				"id", uuidString(s.ID), "src", s.SrcDataset, "err", err)
		}
		if err := m.Queries.MarkReplicationScheduleFired(ctx, storedb.MarkReplicationScheduleFiredParams{
			ID:          s.ID,
			LastFiredAt: pgtype.Timestamptz{Time: now, Valid: true},
		}); err != nil {
			m.Logger.Error("scheduler: mark replication fired failed", "id", uuidString(s.ID), "err", err)
		}
	}
	return nil
}

// fireReplication takes a fresh snapshot, then send/receives it (full or
// incremental) to the configured target, and records the result.
func (m *Manager) fireReplication(ctx context.Context, s storedb.ReplicationSchedule, now time.Time) error {
	target, err := m.Queries.GetReplicationTarget(ctx, s.TargetID)
	if err != nil {
		return fmt.Errorf("get target: %w", err)
	}
	short := FormatSnapTime(s.SnapshotPrefix, now)
	full := s.SrcDataset + "@" + short

	if err := m.Snapshots.Create(ctx, s.SrcDataset, short, false); err != nil {
		return fmt.Errorf("create snapshot %s: %w", full, err)
	}

	// Decide incremental vs full. Incremental is preferred whenever a
	// previous sync snapshot still exists locally.
	var sendOpts dataset.SendOpts
	sendOpts.Compressed = true
	sendOpts.LargeBlock = true
	if s.LastSyncSnapshot != nil && *s.LastSyncSnapshot != "" {
		if exists, err := m.snapshotExists(ctx, *s.LastSyncSnapshot); err != nil {
			return fmt.Errorf("verify last sync snapshot: %w", err)
		} else if exists {
			sendOpts.IncrementalFrom = *s.LastSyncSnapshot
		}
	}

	dst := remoteDataset(target.DatasetPrefix, s.SrcDataset)
	pipe := m.Transport
	if pipe == nil {
		return errors.New("scheduler: no replication transport configured")
	}

	// We compose the pipe by spawning Send into an io.Pipe writer and
	// having Transport.Receive consume the reader. This keeps memory
	// bounded.
	pr, pw := io.Pipe()
	sendCh := make(chan error, 1)
	recvCh := make(chan error, 1)
	go func() {
		err := m.Datasets.Send(ctx, full, sendOpts, pw)
		// CloseWithError lets Receive observe the underlying send error
		// (or clean EOF) instead of getting "io: read/write on closed
		// pipe". Pass nil → close cleanly with EOF.
		_ = pw.CloseWithError(err)
		sendCh <- err
	}()
	go func() {
		err := pipe.Receive(ctx, &target, dst, pr)
		// If Receive bailed early (e.g. SSH refused), close the read
		// side so Send unblocks instead of writing forever into a pipe
		// no one is draining.
		_ = pr.CloseWithError(err)
		recvCh <- err
	}()
	sendErr := <-sendCh
	recvErr := <-recvCh
	// Recv error is the more interesting one (it's the remote-side
	// outcome); fall back to send error if recv was clean.
	if recvErr != nil {
		return recvErr
	}
	if sendErr != nil {
		return sendErr
	}

	if err := m.Queries.MarkReplicationScheduleResult(ctx, storedb.MarkReplicationScheduleResultParams{
		ID:               s.ID,
		LastSyncSnapshot: ptrStr(full),
		LastError:        nil,
	}); err != nil {
		return fmt.Errorf("mark result: %w", err)
	}
	return nil
}

// snapshotExists checks whether name is currently present on the host.
func (m *Manager) snapshotExists(ctx context.Context, name string) (bool, error) {
	at := strings.IndexByte(name, '@')
	if at < 0 {
		return false, fmt.Errorf("not a snapshot name: %q", name)
	}
	ds := name[:at]
	all, err := m.Snapshots.List(ctx, ds)
	if err != nil {
		return false, err
	}
	for _, s := range all {
		if s.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// remoteDataset computes the remote target dataset by joining the
// target's dataset_prefix with the basename of the source. Example:
//
//	prefix="backup/from-tank", src="tank/data" → "backup/from-tank/data"
func remoteDataset(prefix, src string) string {
	base := src
	if i := strings.LastIndexByte(src, '/'); i >= 0 {
		base = src[i+1:]
	}
	return strings.TrimRight(prefix, "/") + "/" + base
}

func ptrStr(s string) *string { return &s }

// uuidString formats a pgtype.UUID into its canonical hex form for logs.
func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	b := id.Bytes
	const hex = "0123456789abcdef"
	out := make([]byte, 36)
	j := 0
	for i, x := range b {
		out[j] = hex[x>>4]
		out[j+1] = hex[x&0x0f]
		j += 2
		if i == 3 || i == 5 || i == 7 || i == 9 {
			out[j] = '-'
			j++
		}
	}
	return string(out)
}
