package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// ErrDuplicate signals that an enqueue collided with an existing task ID
// (uniqueness key already in flight). Callers should map this to HTTP 409.
var ErrDuplicate = errors.New("duplicate dispatch")

type TaskBody struct {
	JobID   string          `json:"jobId"`
	Payload json.RawMessage `json:"payload"`
}

func encodeTaskBody(jobID uuid.UUID, payload json.RawMessage) ([]byte, error) {
	return json.Marshal(TaskBody{JobID: jobID.String(), Payload: payload})
}

type PoolBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Dispatcher struct {
	Client  *asynq.Client
	Queries *storedb.Queries
	Pool    PoolBeginner
}

type DispatchInput struct {
	Kind      Kind
	Target    string
	Payload   any
	Command   string // human-readable cmd preview for the jobs row
	RequestID string
	UniqueKey string // optional; if set, asynq enforces uniqueness
}

type DispatchOutput struct {
	JobID uuid.UUID
}

// Dispatch creates a jobs row and enqueues the corresponding asynq task in
// one logical operation. Ordering is intentional: row insert in tx →
// enqueue → commit. If enqueue fails, the row is rolled back. The
// remaining failure mode (commit fails after enqueue) leaves a phantom
// task with no row; the worker is responsible for tolerating "row not
// found" by acking and logging. We chose this ordering over commit-then-
// enqueue because Postgres outage/timeout is the more common failure and
// keeping the row scoped to the same tx makes "no orphan rows" the
// guarantee that's easier to reason about.
//
// MaxRetry(0): asynq won't retry. We surface failures to the user via the
// jobs row state machine, where retries are an explicit user decision.
//
// asynq.TaskID(uniqueKey): when set, asynq returns ErrTaskIDConflict on a
// duplicate enqueue. We map that to ErrDuplicate so handlers can return
// 409 Conflict.
func (d *Dispatcher) Dispatch(ctx context.Context, in DispatchInput) (DispatchOutput, error) {
	jobID := uuid.New()

	payloadBytes, err := json.Marshal(in.Payload)
	if err != nil {
		return DispatchOutput{}, fmt.Errorf("marshal payload: %w", err)
	}

	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return DispatchOutput{}, err
	}
	// Rollback is a no-op after Commit; this defer covers early returns.
	defer func() { _ = tx.Rollback(ctx) }()
	q := d.Queries.WithTx(tx)

	pgID := pgtype.UUID{Bytes: jobID, Valid: true}

	if _, err := q.InsertJob(ctx, storedb.InsertJobParams{
		ID:        pgID,
		Kind:      string(in.Kind),
		Target:    in.Target,
		Command:   in.Command,
		RequestID: in.RequestID,
	}); err != nil {
		return DispatchOutput{}, err
	}

	body, err := encodeTaskBody(jobID, payloadBytes)
	if err != nil {
		return DispatchOutput{}, err
	}

	opts := []asynq.Option{asynq.MaxRetry(0)}
	if in.UniqueKey != "" {
		// TaskID alone provides caller-keyed dedup; Unique() with the
		// same key is redundant and its ttl=0 semantics are ill-defined.
		opts = append(opts, asynq.TaskID(in.UniqueKey))
	}
	task := asynq.NewTask(string(in.Kind), body, opts...)
	if _, err := d.Client.EnqueueContext(ctx, task); err != nil {
		if errors.Is(err, asynq.ErrTaskIDConflict) {
			return DispatchOutput{}, fmt.Errorf("dispatch %s: %w", in.Kind, ErrDuplicate)
		}
		return DispatchOutput{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DispatchOutput{}, err
	}
	return DispatchOutput{JobID: jobID}, nil
}
