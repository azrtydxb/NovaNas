package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/novanas/nova-nas/internal/replication"
)

// KindReplicationRun is the Asynq task type the new general-replication
// subsystem dispatches. The associated payload is a [ReplicationRunPayload].
//
// We use a stable type string that does not collide with the older
// "scheduler.replication.fire" kind (see scrub/scheduler kinds in
// kind.go). Per-job uniqueness is enforced via a UniqueKey of
// "replication:job:<jobId>" which collapses concurrent runs of the same
// job into a single in-flight dispatch.
const KindReplicationRun Kind = "replication:run"

// ReplicationRunPayload is the on-the-wire body for a replication run.
// Only the JobID is required; everything else is loaded by the worker
// from the database + secrets manager at run time so the payload stays
// minimal and credentials never travel via Asynq.
type ReplicationRunPayload struct {
	JobID uuid.UUID `json:"jobId"`
}

// ReplicationDispatcher is the surface internal/jobs.Dispatcher exposes
// for scheduling replication runs. It mirrors the small DispatcherAPI
// used by scrubpolicy so callers (HTTP handler, scheduler tick) can
// share one path.
type ReplicationDispatcher interface {
	Dispatch(ctx context.Context, in DispatchInput) (DispatchOutput, error)
}

// DispatchReplication enqueues a replication run for jobID. It centralises
// the UniqueKey + Kind wiring so the HTTP /run handler and the scheduler
// tick don't drift. requestID is the audit-correlation id; source is a
// short tag like "api:run" or "scheduler:tick" surfaced to operators.
//
// On a duplicate (asynq returns ErrTaskIDConflict, mapped to ErrDuplicate
// by Dispatcher) the caller should treat the existing in-flight run as
// the truth and return 409 Conflict.
func DispatchReplication(ctx context.Context, d ReplicationDispatcher, jobID uuid.UUID, requestID, source string) (DispatchOutput, error) {
	return d.Dispatch(ctx, DispatchInput{
		Kind:      KindReplicationRun,
		Target:    jobID.String(),
		Payload:   ReplicationRunPayload{JobID: jobID},
		Command:   "replication.run " + jobID.String(),
		RequestID: requestID,
		UniqueKey: "replication:job:" + jobID.String(),
	})
}

// ReplicationRunner is the slice of replication.Manager invoked by the
// Asynq handler. It is an interface so workers can be tested without a
// real backend wired up.
type ReplicationRunner interface {
	Run(ctx context.Context, jobID uuid.UUID) (replication.Run, error)
}

// HandleReplicationRun is the Asynq handler installed on the worker mux
// for KindReplicationRun. It decodes the payload, calls
// ReplicationRunner.Run, and propagates the run error back to Asynq.
//
// The handler is intentionally separate from WorkerDeps' methods in
// worker.go so this package can avoid taking a hard dependency on the
// production replication.Manager from worker.go (which would create an
// import cycle if replication grew to depend on jobs).
func HandleReplicationRun(runner ReplicationRunner) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		if runner == nil {
			return errors.New("replication runner not wired")
		}
		var body TaskBody
		if err := json.Unmarshal(t.Payload(), &body); err != nil {
			return fmt.Errorf("decode task body: %w", err)
		}
		var payload ReplicationRunPayload
		if err := json.Unmarshal(body.Payload, &payload); err != nil {
			return fmt.Errorf("decode replication payload: %w", err)
		}
		if payload.JobID == uuid.Nil {
			return errors.New("replication: empty jobId")
		}
		_, err := runner.Run(ctx, payload.JobID)
		return err
	}
}
