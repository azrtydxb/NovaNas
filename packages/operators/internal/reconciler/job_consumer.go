// Package reconciler — job consumer pattern.
//
// E1 (API-Actions) writes long-running "jobs" to the NovaFlow Drizzle
// jobs table for operations that don't map cleanly onto a CRD
// reconcile (e.g. system.reset, system.supportBundle,
// system.checkUpdate, snapshot.restore). The operator-side half of
// that contract is a JobConsumer: one or more goroutines polling the
// queue for jobs of a known kind, dispatching to a registered
// handler, and updating the job record with success/failure.
//
// This file provides the dispatch scaffolding. The queue backend is
// abstracted behind JobsBackend so production deployments can plug in
// a real DB/HTTP client without changing the controller wiring. The
// default backend is NoopJobsBackend which never yields a job — safe
// for dev clusters that haven't wired the API yet.
package reconciler

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

// JobRecord is the operator-visible view of a row in the jobs table.
// Fields mirror the minimum subset of the Drizzle schema the
// consumers actually need; additional fields live opaquely in Input.
type JobRecord struct {
	ID    string
	Kind  string
	State string
	Input map[string]any
}

// JobResult is returned by a handler and recorded against the job row.
// Success=false leaves the job in state="failed" with Message as the
// user-visible error.
type JobResult struct {
	Success bool
	Message string
	Result  map[string]any
}

// JobHandler performs the work for a single job kind. Handlers should
// be idempotent — the consumer may invoke the same job twice if a
// backend write is lost between "claim" and "complete".
type JobHandler func(ctx context.Context, job JobRecord) JobResult

// JobsBackend is the queue abstraction the JobConsumer polls. A
// production implementation bridges to the NovaFlow API (or directly
// to the Drizzle DB); the default NoopJobsBackend is a no-op so the
// operator can start cleanly without that dependency wired.
type JobsBackend interface {
	// ClaimNext atomically transitions one queued job of the given
	// kind to "running" and returns it. Returns (zero, nil) when no
	// job is available.
	ClaimNext(ctx context.Context, kind string) (JobRecord, bool, error)
	// Complete marks the job as succeeded or failed and records the
	// handler's result payload.
	Complete(ctx context.Context, id string, result JobResult) error
}

// NoopJobsBackend never yields a job. Used when the operator is
// running without a wired jobs API.
type NoopJobsBackend struct{}

// ClaimNext always returns (zero, false, nil).
func (NoopJobsBackend) ClaimNext(_ context.Context, _ string) (JobRecord, bool, error) {
	return JobRecord{}, false, nil
}

// Complete is a no-op.
func (NoopJobsBackend) Complete(_ context.Context, _ string, _ JobResult) error { return nil }

// JobConsumer dispatches queued jobs to registered handlers. A single
// consumer hosts N handlers, one per kind; Start spawns one poll
// goroutine per registered kind.
type JobConsumer struct {
	Backend  JobsBackend
	Log      logr.Logger
	Interval time.Duration

	mu       sync.Mutex
	handlers map[string]JobHandler
}

// NewJobConsumer builds a consumer with the given backend. A nil
// backend is replaced with NoopJobsBackend so the consumer is always
// safe to Start.
func NewJobConsumer(b JobsBackend, log logr.Logger) *JobConsumer {
	if b == nil {
		b = NoopJobsBackend{}
	}
	return &JobConsumer{
		Backend:  b,
		Log:      log,
		Interval: 5 * time.Second,
		handlers: map[string]JobHandler{},
	}
}

// Register binds a handler to a job kind. Subsequent registrations
// for the same kind overwrite the prior handler.
func (c *JobConsumer) Register(kind string, h JobHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.handlers == nil {
		c.handlers = map[string]JobHandler{}
	}
	c.handlers[kind] = h
}

// Start launches one goroutine per registered kind. It returns nil
// immediately; goroutines exit when ctx is cancelled. Safe to call
// exactly once.
func (c *JobConsumer) Start(ctx context.Context) error {
	c.mu.Lock()
	kinds := make([]string, 0, len(c.handlers))
	for k := range c.handlers {
		kinds = append(kinds, k)
	}
	c.mu.Unlock()
	for _, kind := range kinds {
		go c.runKind(ctx, kind)
	}
	return nil
}

func (c *JobConsumer) runKind(ctx context.Context, kind string) {
	log := c.Log.WithValues("jobKind", kind)
	log.V(1).Info("job consumer started")
	iv := c.Interval
	if iv <= 0 {
		iv = 5 * time.Second
	}
	ticker := time.NewTicker(iv)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.V(1).Info("job consumer stopping")
			return
		case <-ticker.C:
			c.pollOnce(ctx, kind, log)
		}
	}
}

func (c *JobConsumer) pollOnce(ctx context.Context, kind string, log logr.Logger) {
	job, ok, err := c.Backend.ClaimNext(ctx, kind)
	if err != nil {
		log.V(1).Info("ClaimNext failed", "error", err.Error())
		return
	}
	if !ok {
		return
	}
	c.mu.Lock()
	h := c.handlers[kind]
	c.mu.Unlock()
	if h == nil {
		log.Info("no handler registered; skipping", "jobID", job.ID)
		return
	}
	log.V(1).Info("dispatching job", "jobID", job.ID)
	// Recover from handler panics so one bad job doesn't kill the
	// consumer goroutine.
	result := func() (r JobResult) {
		defer func() {
			if rec := recover(); rec != nil {
				r = JobResult{Success: false, Message: "handler panic"}
			}
		}()
		return h(ctx, job)
	}()
	if err := c.Backend.Complete(ctx, job.ID, result); err != nil {
		log.V(1).Info("Complete failed", "jobID", job.ID, "error", err.Error())
	}
}
