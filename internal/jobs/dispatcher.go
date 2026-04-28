package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

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
		opts = append(opts, asynq.Unique(0))
		opts = append(opts, asynq.TaskID(in.UniqueKey))
	}
	task := asynq.NewTask(string(in.Kind), body, opts...)
	if _, err := d.Client.EnqueueContext(ctx, task); err != nil {
		return DispatchOutput{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DispatchOutput{}, err
	}
	return DispatchOutput{JobID: jobID}, nil
}
