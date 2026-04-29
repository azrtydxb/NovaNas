package replication

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Store is the persistence boundary for [Manager]. The production
// implementation lives alongside the rest of the sqlc-generated code;
// tests pass an in-memory implementation.
//
// Implementations must be safe for concurrent use.
type Store interface {
	CreateJob(ctx context.Context, j Job) (Job, error)
	UpdateJob(ctx context.Context, j Job) (Job, error)
	DeleteJob(ctx context.Context, id uuid.UUID) error
	GetJob(ctx context.Context, id uuid.UUID) (Job, error)
	ListJobs(ctx context.Context) ([]Job, error)

	CreateRun(ctx context.Context, r Run) (Run, error)
	UpdateRun(ctx context.Context, r Run) (Run, error)
	ListRuns(ctx context.Context, jobID uuid.UUID, limit int) ([]Run, error)
}

// Locker is the distributed lock used to enforce "at most one in-flight
// run per job". The production implementation wraps Redis SETNX; the
// in-memory implementation in this package's tests uses a sync.Map.
type Locker interface {
	// TryLock attempts to acquire an exclusive lock keyed by id. Returns
	// (true, releaseFn, nil) on success. The release function is safe
	// to call multiple times.
	TryLock(ctx context.Context, id uuid.UUID, ttl time.Duration) (locked bool, release func(), err error)
}

// ErrNotFound is returned by [Manager] methods that target a missing job.
var ErrNotFound = errors.New("replication: job not found")

// ErrLocked is returned by [Manager.Run] when a run is already in
// flight for the same job.
var ErrLocked = errors.New("replication: another run is already in progress")

// ManagerOptions bundle the optional dependencies for [NewManager].
type ManagerOptions struct {
	Logger *slog.Logger
	Now    func() time.Time
	// LockTTL caps how long a run can hold the per-job lock. Defaults
	// to 6 hours.
	LockTTL time.Duration
}

// Manager is the public façade of the replication subsystem.
type Manager struct {
	store    Store
	locker   Locker
	backends map[BackendKind]Backend
	logger   *slog.Logger
	now      func() time.Time
	lockTTL  time.Duration
}

// NewManager wires a Manager. backends must contain at least one
// backend; backends not registered will reject jobs that select them.
func NewManager(store Store, locker Locker, backends []Backend, opts ManagerOptions) *Manager {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	ttl := opts.LockTTL
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	bmap := make(map[BackendKind]Backend, len(backends))
	for _, b := range backends {
		bmap[b.Kind()] = b
	}
	return &Manager{
		store:    store,
		locker:   locker,
		backends: bmap,
		logger:   logger,
		now:      now,
		lockTTL:  ttl,
	}
}

// Create persists a new replication job after running both generic and
// backend-specific validation. The returned Job has its server-assigned
// fields populated.
func (m *Manager) Create(ctx context.Context, j Job) (Job, error) {
	if err := j.Validate(); err != nil {
		return Job{}, err
	}
	b, ok := m.backends[j.Backend]
	if !ok {
		return Job{}, fmt.Errorf("replication: backend %q is not registered", j.Backend)
	}
	if err := b.Validate(ctx, j); err != nil {
		return Job{}, err
	}
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	now := m.now().UTC()
	if j.CreatedAt.IsZero() {
		j.CreatedAt = now
	}
	j.UpdatedAt = now
	return m.store.CreateJob(ctx, j)
}

// Update applies a partial-or-full job update. The caller is
// responsible for merging fields onto the latest version retrieved via
// [Manager.Get]; this method simply re-validates and persists.
func (m *Manager) Update(ctx context.Context, j Job) (Job, error) {
	if err := j.Validate(); err != nil {
		return Job{}, err
	}
	b, ok := m.backends[j.Backend]
	if !ok {
		return Job{}, fmt.Errorf("replication: backend %q is not registered", j.Backend)
	}
	if err := b.Validate(ctx, j); err != nil {
		return Job{}, err
	}
	j.UpdatedAt = m.now().UTC()
	return m.store.UpdateJob(ctx, j)
}

// Delete removes a job and its on-disk state. Removing the matching
// scheduler entry / OpenBao secrets is the caller's responsibility.
func (m *Manager) Delete(ctx context.Context, id uuid.UUID) error {
	return m.store.DeleteJob(ctx, id)
}

// Get returns a single job by ID.
func (m *Manager) Get(ctx context.Context, id uuid.UUID) (Job, error) {
	return m.store.GetJob(ctx, id)
}

// List returns all jobs.
func (m *Manager) List(ctx context.Context) ([]Job, error) {
	return m.store.ListJobs(ctx)
}

// Runs returns the most recent N runs for a job, newest first.
func (m *Manager) Runs(ctx context.Context, jobID uuid.UUID, limit int) ([]Run, error) {
	return m.store.ListRuns(ctx, jobID, limit)
}

// Run executes a single replication pass for jobID synchronously. Use
// it from the Asynq worker (the schedule-driven path) and from the
// /run HTTP handler (ad-hoc trigger).
//
// Concurrency: at most one run per job. If another run holds the lock,
// Run returns ErrLocked without creating a Run record.
func (m *Manager) Run(ctx context.Context, jobID uuid.UUID) (Run, error) {
	job, err := m.store.GetJob(ctx, jobID)
	if err != nil {
		return Run{}, err
	}
	b, ok := m.backends[job.Backend]
	if !ok {
		return Run{}, fmt.Errorf("replication: backend %q is not registered", job.Backend)
	}

	locked, release, err := m.locker.TryLock(ctx, job.ID, m.lockTTL)
	if err != nil {
		return Run{}, fmt.Errorf("replication: acquire lock: %w", err)
	}
	if !locked {
		return Run{}, ErrLocked
	}
	defer release()

	start := m.now().UTC()
	run := Run{
		ID:        uuid.New(),
		JobID:     job.ID,
		StartedAt: start,
		Outcome:   RunRunning,
	}
	run, err = m.store.CreateRun(ctx, run)
	if err != nil {
		return Run{}, fmt.Errorf("replication: create run: %w", err)
	}

	res, execErr := b.Execute(ctx, ExecuteContext{Job: job})

	end := m.now().UTC()
	run.FinishedAt = &end
	run.BytesTransferred = res.BytesTransferred
	run.Snapshot = res.Snapshot
	if execErr != nil {
		run.Outcome = RunFailed
		run.Error = execErr.Error()
		m.logger.Error("replication run failed",
			"jobId", job.ID.String(), "name", job.Name,
			"backend", job.Backend, "err", execErr)
	} else {
		run.Outcome = RunSucceeded
		// Persist the new last-snapshot pointer for incremental ZFS.
		if job.Backend == BackendZFS && res.Snapshot != "" {
			job.LastSnapshot = res.Snapshot
			job.UpdatedAt = end
			if _, uerr := m.store.UpdateJob(ctx, job); uerr != nil {
				m.logger.Warn("replication: persist last snapshot failed",
					"jobId", job.ID.String(), "err", uerr)
			}
		}
	}
	if _, err := m.store.UpdateRun(ctx, run); err != nil {
		return run, fmt.Errorf("replication: update run: %w", err)
	}
	return run, execErr
}

// SecretsManager is the secret-storage facade [New] uses to construct
// per-run backends. It is intentionally a tiny interface so tests can
// pass a fake without depending on internal/host/secrets.
type SecretsManager interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// AsynqClient is the slice of *asynq.Client this package needs. Callers
// can pass *asynq.Client directly. The interface keeps internal/replication
// from importing the concrete asynq dependency at top level.
type AsynqClient interface{}

// New is a convenience constructor that wires Manager with the
// production dependencies. Backends are still passed explicitly because
// their concrete construction (e.g. zfs vs s3 vs rsync) varies per run
// and is not the manager's concern. The asynq client and secrets
// manager are accepted but not used by the manager itself — they are
// here for symmetry with the rest of the codebase's "constructor takes
// every dep" idiom; the worker is the place that resolves secrets and
// builds backends per run (see internal/jobs/replication_task.go).
//
// Tests should keep using NewManager + NewMemStore + NewMemLocker.
func New(store Store, locker Locker, backends []Backend, opts ManagerOptions, _ SecretsManager, _ AsynqClient) *Manager {
	return NewManager(store, locker, backends, opts)
}

// ----- in-memory implementations for tests / dev -----

// MemStore is a Store implementation that holds everything in memory.
// It is safe for concurrent use and useful for tests and for early
// development before the SQL schema lands.
type MemStore struct {
	mu   sync.Mutex
	jobs map[uuid.UUID]Job
	runs map[uuid.UUID][]Run
}

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{
		jobs: map[uuid.UUID]Job{},
		runs: map[uuid.UUID][]Run{},
	}
}

func (s *MemStore) CreateJob(_ context.Context, j Job) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[j.ID]; exists {
		return Job{}, fmt.Errorf("replication: job %s already exists", j.ID)
	}
	s.jobs[j.ID] = j
	return j, nil
}

func (s *MemStore) UpdateJob(_ context.Context, j Job) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[j.ID]; !exists {
		return Job{}, ErrNotFound
	}
	s.jobs[j.ID] = j
	return j, nil
}

func (s *MemStore) DeleteJob(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[id]; !exists {
		return ErrNotFound
	}
	delete(s.jobs, id)
	delete(s.runs, id)
	return nil
}

func (s *MemStore) GetJob(_ context.Context, id uuid.UUID) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	return j, nil
}

func (s *MemStore) ListJobs(_ context.Context) ([]Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j)
	}
	return out, nil
}

func (s *MemStore) CreateRun(_ context.Context, r Run) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[r.JobID] = append(s.runs[r.JobID], r)
	return r, nil
}

func (s *MemStore) UpdateRun(_ context.Context, r Run) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := s.runs[r.JobID]
	for i := range runs {
		if runs[i].ID == r.ID {
			runs[i] = r
			s.runs[r.JobID] = runs
			return r, nil
		}
	}
	return Run{}, ErrNotFound
}

func (s *MemStore) ListRuns(_ context.Context, jobID uuid.UUID, limit int) ([]Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := s.runs[jobID]
	// Newest first.
	out := make([]Run, len(runs))
	for i := range runs {
		out[len(runs)-1-i] = runs[i]
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// MemLocker is a Locker that uses an in-process map. Suitable for
// tests and for single-replica deployments where Redis isn't
// available; a production deployment should use a Redis-backed locker.
type MemLocker struct {
	mu   sync.Mutex
	held map[uuid.UUID]struct{}
}

// NewMemLocker constructs an empty MemLocker.
func NewMemLocker() *MemLocker {
	return &MemLocker{held: map[uuid.UUID]struct{}{}}
}

// TryLock implements Locker.
func (l *MemLocker) TryLock(_ context.Context, id uuid.UUID, _ time.Duration) (bool, func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.held[id]; exists {
		return false, func() {}, nil
	}
	l.held[id] = struct{}{}
	released := false
	return true, func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if released {
			return
		}
		released = true
		delete(l.held, id)
	}, nil
}
