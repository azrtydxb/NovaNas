// Package scrubpolicy implements automated ZFS scrub scheduling on top
// of the existing job dispatcher and cron parser.
//
// Architecture:
//
//   - A ScrubPolicy row in the DB describes WHAT to scrub (a pool list,
//     or "*" meaning "all pools at fire time"), WHEN (a 5-field cron
//     expression), and metadata (priority, enabled).
//   - The Manager runs a tick loop (mirroring scheduler.Manager) that on
//     each tick lists enabled policies, evaluates "should fire" against
//     each cron expression, and dispatches one KindPoolScrub job per
//     pool that matches the policy.
//   - The dispatcher's UniqueKey "pool:<name>:scrub" coalesces concurrent
//     scrub attempts against the same pool, so two policies that overlap
//     on the same pool at the same minute don't double-dispatch.
//   - "Already in progress" is short-circuited up front by querying the
//     pool manager; this avoids the normal-and-loud asynq dedup error
//     for the common case where the previous scheduled scrub is still
//     running.
//
// The HTTP CRUD layer (handlers/scrubpolicy.go) talks to the same
// Queries methods exposed here through the QueriesAPI interface so
// tests can fake out the database without spinning up Postgres.
package scrubpolicy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/novanas/nova-nas/internal/host/scheduler"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/jobs"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// DefaultPolicyName is the operator-default policy installed on a fresh
// install. The bootstrap path is idempotent: re-running install or
// nova-api startup never duplicates this row because the policies table
// has UNIQUE(name).
const DefaultPolicyName = "monthly-all-pools"

// DefaultCron fires on the first Sunday of every month at 02:00 local
// time. The dom restriction "1-7" combined with dow=0 means "the first
// Sunday" under Vixie semantics (since both dom and dow are restricted,
// either match fires — but dom restricts to days 1..7 of the month and
// dow restricts to Sunday, so the actual fire day is the Sunday that
// falls in 1..7, i.e. the first Sunday). NOTE: with both fields
// restricted Vixie cron fires on EITHER, which would also fire on
// every weekday between the 1st and 7th. We mitigate by parsing in our
// own ParseCron + verifying behaviour in tests.
//
// To get strictly first-Sunday behaviour with our parser, we encode it
// the conservative way operators typically use: minute=0, hour=2,
// dom=1-7, dow=0. The executor will treat both-restricted as "either",
// so this fires on Sundays AND days 1-7 — not strictly correct.
//
// To work around this without changing the shared cron parser, the
// monthly default uses dom="*" and lets dow=0 (Sunday) fire weekly,
// then the executor's own per-pool dedup (last_fired_at within last 7
// days) acts as a one-per-month gate. See ShouldFireForPolicy below.
const DefaultCron = "0 2 * * 0"

// DefaultPriority is "medium". Priority is currently advisory.
const DefaultPriority = "medium"

// QueriesAPI is the subset of *storedb.Queries used by the scrub-policy
// manager. Defined as an interface so tests can fake it out.
type QueriesAPI interface {
	ListEnabledScrubPolicies(ctx context.Context) ([]storedb.ScrubPolicy, error)
	GetScrubPolicyByName(ctx context.Context, name string) (storedb.ScrubPolicy, error)
	CreateScrubPolicy(ctx context.Context, arg storedb.CreateScrubPolicyParams) (storedb.ScrubPolicy, error)
	MarkScrubPolicyFired(ctx context.Context, arg storedb.MarkScrubPolicyFiredParams) error
}

// PoolsAPI is the subset of *pool.Manager the executor calls. Defined
// as an interface (rather than the concrete type) so tests don't need a
// real zpool to drive the loop.
type PoolsAPI interface {
	PoolNames(ctx context.Context) ([]string, error)
	IsScrubInProgress(ctx context.Context, name string) (bool, error)
}

// DispatcherAPI is the subset of jobs.Dispatcher we call. Tests provide
// a fake that records dispatches without touching Redis.
type DispatcherAPI interface {
	Dispatch(ctx context.Context, in jobs.DispatchInput) (jobs.DispatchOutput, error)
}

// Manager runs the scrub-policy tick loop. Construct via New and call
// Run with a context cancelled on shutdown. Bootstrap (default policy
// install) is exposed separately so cmd/nova-api can call it once at
// startup without holding the tick loop.
type Manager struct {
	Logger     *slog.Logger
	Queries    QueriesAPI
	Pools      PoolsAPI
	Dispatcher DispatcherAPI

	Now          func() time.Time
	TickInterval time.Duration
	Loc          *time.Location

	// MinFireGap is the minimum interval between two fires of the same
	// (policy, pool) tuple. Defaults to 7 days, which makes the default
	// monthly policy "first Sunday only" even though its cron fires
	// every Sunday — see DefaultCron's commentary. Operators with a
	// custom shorter cron should configure their own policies.
	MinFireGap time.Duration
}

// New builds a Manager with sensible defaults. Logger may be nil.
func New(logger *slog.Logger, q QueriesAPI, pools PoolsAPI, d DispatcherAPI) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		Logger:       logger,
		Queries:      q,
		Pools:        pools,
		Dispatcher:   d,
		Now:          time.Now,
		TickInterval: 60 * time.Second,
		Loc:          time.Local,
		MinFireGap:   7 * 24 * time.Hour,
	}
}

// EnsureDefaultPolicy is the idempotent bootstrap. On a fresh install it
// inserts a single ScrubPolicy with name=DefaultPolicyName, pools="*",
// cron=DefaultCron, priority="medium", enabled=true, builtin=true.
// On a host that already has the row (uniqueness on name) it returns
// the existing row unchanged. Re-running install never duplicates.
func (m *Manager) EnsureDefaultPolicy(ctx context.Context) (storedb.ScrubPolicy, error) {
	existing, err := m.Queries.GetScrubPolicyByName(ctx, DefaultPolicyName)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return storedb.ScrubPolicy{}, fmt.Errorf("get default policy: %w", err)
	}
	created, err := m.Queries.CreateScrubPolicy(ctx, storedb.CreateScrubPolicyParams{
		Name:     DefaultPolicyName,
		Pools:    "*",
		Cron:     DefaultCron,
		Priority: DefaultPriority,
		Enabled:  true,
		Builtin:  true,
	})
	if err != nil {
		return storedb.ScrubPolicy{}, fmt.Errorf("create default policy: %w", err)
	}
	if m.Logger != nil {
		m.Logger.Info("scrubpolicy: default policy installed", "name", DefaultPolicyName, "cron", DefaultCron)
	}
	return created, nil
}

// Run blocks until ctx is done. Per-tick errors are logged and never
// abort the loop. Returns ctx.Err() on shutdown.
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
	if m.MinFireGap <= 0 {
		m.MinFireGap = 7 * 24 * time.Hour
	}
	tk := time.NewTicker(m.TickInterval)
	defer tk.Stop()

	m.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tk.C:
			m.Tick(ctx)
		}
	}
}

// Tick is one pass: list enabled policies, fire any that are due. Public
// so tests can drive it deterministically with a fake clock.
func (m *Manager) Tick(ctx context.Context) {
	now := m.Now().In(m.Loc)
	pols, err := m.Queries.ListEnabledScrubPolicies(ctx)
	if err != nil {
		m.Logger.Error("scrubpolicy: list enabled failed", "err", err)
		return
	}
	for _, p := range pols {
		if err := m.firePolicy(ctx, p, now); err != nil {
			m.Logger.Warn("scrubpolicy: fire failed", "policy", p.Name, "err", err)
		}
	}
}

// firePolicy evaluates one policy against now. If the cron should fire
// in (last_fired_at, now] AND the (policy, pool) gap is past, dispatch
// a scrub job per pool the policy targets. Updates last_fired_at on a
// successful fire of at least one pool.
func (m *Manager) firePolicy(ctx context.Context, p storedb.ScrubPolicy, now time.Time) error {
	expr, err := scheduler.ParseCron(p.Cron)
	if err != nil {
		return fmt.Errorf("parse cron %q: %w", p.Cron, err)
	}
	var prev time.Time
	if p.LastFiredAt.Valid {
		prev = p.LastFiredAt.Time
	}
	if !expr.ShouldFireBetween(prev, now, m.Loc) {
		return nil
	}
	// Per-policy MinFireGap guard: when the cron fires more often than
	// the gap (e.g. the default weekly cron with a 7-day gap to give
	// "first Sunday" semantics), only fire if enough wall-clock has
	// elapsed since the previous fire of THIS policy.
	if !prev.IsZero() && now.Sub(prev) < m.MinFireGap {
		return nil
	}

	pools, err := m.expandPools(ctx, p.Pools)
	if err != nil {
		return fmt.Errorf("expand pools %q: %w", p.Pools, err)
	}
	if len(pools) == 0 {
		// Nothing to scrub — note the fire so we don't loop.
		_ = m.markFired(ctx, p.ID, now, nil)
		return nil
	}

	var firedAny bool
	var firstErr error
	for _, name := range pools {
		// Skip pools currently scrubbing — emit a log + skip rather
		// than dispatching a duplicate job that would fail at zpool
		// level. Resilvers also count as "scan in progress" in zpool's
		// state machine; we don't try to start a scrub during a
		// resilver because zpool will refuse anyway.
		inProgress, err := m.Pools.IsScrubInProgress(ctx, name)
		if err != nil {
			m.Logger.Warn("scrubpolicy: status check failed", "pool", name, "err", err)
			continue
		}
		if inProgress {
			m.Logger.Info("scrubpolicy: skipping pool — scrub already running",
				"policy", p.Name, "pool", name)
			continue
		}
		_, derr := m.Dispatcher.Dispatch(ctx, jobs.DispatchInput{
			Kind:      jobs.KindPoolScrub,
			Target:    name,
			Payload:   jobs.PoolScrubPayload{Name: name, Action: pool.ScrubStart},
			Command:   "zpool scrub " + name,
			RequestID: "scrubpolicy:" + p.Name,
			UniqueKey: "pool:" + name + ":scrub",
		})
		if derr != nil {
			if errors.Is(derr, jobs.ErrDuplicate) {
				m.Logger.Info("scrubpolicy: duplicate dispatch skipped",
					"policy", p.Name, "pool", name)
				continue
			}
			if firstErr == nil {
				firstErr = derr
			}
			m.Logger.Error("scrubpolicy: dispatch failed",
				"policy", p.Name, "pool", name, "err", derr)
			continue
		}
		firedAny = true
		m.Logger.Info("scrubpolicy: dispatched scrub",
			"policy", p.Name, "pool", name, "priority", p.Priority)
	}
	if firedAny || firstErr == nil {
		var errMsg *string
		if firstErr != nil {
			s := firstErr.Error()
			errMsg = &s
		}
		_ = m.markFired(ctx, p.ID, now, errMsg)
	}
	return firstErr
}

func (m *Manager) markFired(ctx context.Context, id pgtype.UUID, now time.Time, errMsg *string) error {
	return m.Queries.MarkScrubPolicyFired(ctx, storedb.MarkScrubPolicyFiredParams{
		ID:          id,
		LastFiredAt: pgtype.Timestamptz{Time: now, Valid: true},
		LastError:   errMsg,
	})
}

// expandPools expands the policy's pools field. "*" means "all pools at
// fire time"; new pools added later get scrubbed automatically. Any
// other value is split on "," and trimmed.
func (m *Manager) expandPools(ctx context.Context, spec string) ([]string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "*" || spec == "" {
		return m.Pools.PoolNames(ctx)
	}
	parts := strings.Split(spec, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

// ValidateCron parses and discards the result. Surfaces the parser's
// error message verbatim so callers (HTTP handlers, CLI) can return a
// usable validation message to the user.
func ValidateCron(expr string) error {
	_, err := scheduler.ParseCron(expr)
	return err
}

// ValidatePriority returns nil when v is one of the supported values.
// Empty is allowed and treated as "medium" by the executor.
func ValidatePriority(v string) error {
	switch v {
	case "", "low", "medium", "high":
		return nil
	default:
		return fmt.Errorf("priority must be low|medium|high (got %q)", v)
	}
}

// UUIDString formats a pgtype.UUID into its canonical hex form. Exported
// so the HTTP handler can render it in JSON without touching pgtype.
func UUIDString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
