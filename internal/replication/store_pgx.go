package replication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// PgxQueries is the subset of *storedb.Queries that PgxStore uses.
// Defining it as an interface lets tests substitute a fake without
// hitting a real database.
type PgxQueries interface {
	CreateReplicationJob(ctx context.Context, arg storedb.CreateReplicationJobParams) (storedb.ReplicationJob, error)
	UpdateReplicationJob(ctx context.Context, arg storedb.UpdateReplicationJobParams) (storedb.ReplicationJob, error)
	DeleteReplicationJob(ctx context.Context, id pgtype.UUID) error
	GetReplicationJob(ctx context.Context, id pgtype.UUID) (storedb.ReplicationJob, error)
	ListReplicationJobs(ctx context.Context) ([]storedb.ReplicationJob, error)
	ListEnabledReplicationJobs(ctx context.Context) ([]storedb.ReplicationJob, error)
	MarkReplicationJobFired(ctx context.Context, arg storedb.MarkReplicationJobFiredParams) error

	InsertReplicationRun(ctx context.Context, arg storedb.InsertReplicationRunParams) (storedb.ReplicationRun, error)
	UpdateReplicationRun(ctx context.Context, arg storedb.UpdateReplicationRunParams) (storedb.ReplicationRun, error)
	ListReplicationRuns(ctx context.Context, arg storedb.ListReplicationRunsParams) ([]storedb.ReplicationRun, error)
	ListReplicationRunsAfter(ctx context.Context, arg storedb.ListReplicationRunsAfterParams) ([]storedb.ReplicationRun, error)
}

// PgxStore is the production [Store] implementation backed by sqlc-
// generated pgx queries. It encodes the structured Source / Destination
// / Retention payloads as JSONB columns and decodes them on read.
type PgxStore struct {
	Q PgxQueries
}

// NewPgxStore wires a Store using the supplied queries handle.
func NewPgxStore(q PgxQueries) *PgxStore { return &PgxStore{Q: q} }

var _ Store = (*PgxStore)(nil)

// CreateJob persists a new job. Caller must populate j.ID (Manager.Create
// already does so).
func (s *PgxStore) CreateJob(ctx context.Context, j Job) (Job, error) {
	src, dst, ret, err := encodeJobJSON(j)
	if err != nil {
		return Job{}, err
	}
	row, err := s.Q.CreateReplicationJob(ctx, storedb.CreateReplicationJobParams{
		ID:              pgtype.UUID{Bytes: j.ID, Valid: true},
		Name:            j.Name,
		Backend:         string(j.Backend),
		Direction:       string(j.Direction),
		SourceJson:      src,
		DestinationJson: dst,
		Schedule:        j.Schedule,
		RetentionJson:   ret,
		Enabled:         j.Enabled,
		SecretRef:       j.SecretRef,
		LastSnapshot:    j.LastSnapshot,
	})
	if err != nil {
		return Job{}, err
	}
	return rowToJob(row)
}

// UpdateJob replaces the mutable fields on an existing job.
func (s *PgxStore) UpdateJob(ctx context.Context, j Job) (Job, error) {
	src, dst, ret, err := encodeJobJSON(j)
	if err != nil {
		return Job{}, err
	}
	row, err := s.Q.UpdateReplicationJob(ctx, storedb.UpdateReplicationJobParams{
		ID:              pgtype.UUID{Bytes: j.ID, Valid: true},
		Name:            j.Name,
		Backend:         string(j.Backend),
		Direction:       string(j.Direction),
		SourceJson:      src,
		DestinationJson: dst,
		Schedule:        j.Schedule,
		RetentionJson:   ret,
		Enabled:         j.Enabled,
		SecretRef:       j.SecretRef,
		LastSnapshot:    j.LastSnapshot,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, ErrNotFound
		}
		return Job{}, err
	}
	return rowToJob(row)
}

// DeleteJob removes a job and (via ON DELETE CASCADE) its run history.
func (s *PgxStore) DeleteJob(ctx context.Context, id uuid.UUID) error {
	return s.Q.DeleteReplicationJob(ctx, pgtype.UUID{Bytes: id, Valid: true})
}

// GetJob returns one job. Maps pgx.ErrNoRows to ErrNotFound.
func (s *PgxStore) GetJob(ctx context.Context, id uuid.UUID) (Job, error) {
	row, err := s.Q.GetReplicationJob(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, ErrNotFound
		}
		return Job{}, err
	}
	return rowToJob(row)
}

// ListJobs returns all jobs.
func (s *PgxStore) ListJobs(ctx context.Context) ([]Job, error) {
	rows, err := s.Q.ListReplicationJobs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(rows))
	for _, r := range rows {
		j, err := rowToJob(r)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

// ListEnabledJobs is used by the tick scheduler in cmd/nova-api.
func (s *PgxStore) ListEnabledJobs(ctx context.Context) ([]Job, error) {
	rows, err := s.Q.ListEnabledReplicationJobs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(rows))
	for _, r := range rows {
		j, err := rowToJob(r)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

// MarkFired updates last_fired_at on a job. Used by the scheduler tick.
func (s *PgxStore) MarkFired(ctx context.Context, id uuid.UUID, ts pgtype.Timestamptz) error {
	return s.Q.MarkReplicationJobFired(ctx, storedb.MarkReplicationJobFiredParams{
		ID:          pgtype.UUID{Bytes: id, Valid: true},
		LastFiredAt: ts,
	})
}

// CreateRun inserts the initial Run row. Caller pre-populates ID.
func (s *PgxStore) CreateRun(ctx context.Context, r Run) (Run, error) {
	startedAt := pgtype.Timestamptz{Time: r.StartedAt, Valid: true}
	row, err := s.Q.InsertReplicationRun(ctx, storedb.InsertReplicationRunParams{
		ID:               pgtype.UUID{Bytes: r.ID, Valid: true},
		JobID:            pgtype.UUID{Bytes: r.JobID, Valid: true},
		StartedAt:        startedAt,
		Outcome:          string(r.Outcome),
		BytesTransferred: r.BytesTransferred,
		Snapshot:         r.Snapshot,
		Error:            r.Error,
	})
	if err != nil {
		return Run{}, err
	}
	return rowToRun(row), nil
}

// UpdateRun stamps the terminal outcome (success/failure) onto a run row.
func (s *PgxStore) UpdateRun(ctx context.Context, r Run) (Run, error) {
	finished := pgtype.Timestamptz{}
	if r.FinishedAt != nil {
		finished = pgtype.Timestamptz{Time: *r.FinishedAt, Valid: true}
	}
	row, err := s.Q.UpdateReplicationRun(ctx, storedb.UpdateReplicationRunParams{
		ID:               pgtype.UUID{Bytes: r.ID, Valid: true},
		FinishedAt:       finished,
		Outcome:          string(r.Outcome),
		BytesTransferred: r.BytesTransferred,
		Snapshot:         r.Snapshot,
		Error:            r.Error,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Run{}, ErrNotFound
		}
		return Run{}, err
	}
	return rowToRun(row), nil
}

// ListRuns returns the most recent N runs for a job.
func (s *PgxStore) ListRuns(ctx context.Context, jobID uuid.UUID, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.Q.ListReplicationRuns(ctx, storedb.ListReplicationRunsParams{
		JobID: pgtype.UUID{Bytes: jobID, Valid: true},
		Limit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Run, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToRun(r))
	}
	return out, nil
}

// ListRunsAfter is used for cursor-based pagination of run history.
func (s *PgxStore) ListRunsAfter(ctx context.Context, jobID uuid.UUID, ts pgtype.Timestamptz, _ uuid.UUID, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 50
	}
	// pgx does not let us bind a uuid to the (started_at, id) tuple comparator
	// directly; instead we re-use ts on both sides since strict-less by
	// started_at alone is sufficient for monotonic clocks and we keep id
	// in the ORDER BY for tie-break stability.
	rows, err := s.Q.ListReplicationRunsAfter(ctx, storedb.ListReplicationRunsAfterParams{
		JobID:       pgtype.UUID{Bytes: jobID, Valid: true},
		StartedAt:   ts,
		StartedAt_2: ts,
		Limit:       int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Run, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToRun(r))
	}
	return out, nil
}

// ----- helpers -----

func encodeJobJSON(j Job) (src, dst, ret []byte, err error) {
	src, err = json.Marshal(j.Source)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal source: %w", err)
	}
	dst, err = json.Marshal(j.Destination)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal destination: %w", err)
	}
	ret, err = json.Marshal(j.Retention)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal retention: %w", err)
	}
	return src, dst, ret, nil
}

func rowToJob(r storedb.ReplicationJob) (Job, error) {
	j := Job{
		ID:           uuid.UUID(r.ID.Bytes),
		Name:         r.Name,
		Backend:      BackendKind(r.Backend),
		Direction:    Direction(r.Direction),
		Schedule:     r.Schedule,
		Enabled:      r.Enabled,
		SecretRef:    r.SecretRef,
		LastSnapshot: r.LastSnapshot,
	}
	if r.CreatedAt.Valid {
		j.CreatedAt = r.CreatedAt.Time
	}
	if r.UpdatedAt.Valid {
		j.UpdatedAt = r.UpdatedAt.Time
	}
	if len(r.SourceJson) > 0 {
		if err := json.Unmarshal(r.SourceJson, &j.Source); err != nil {
			return Job{}, fmt.Errorf("unmarshal source: %w", err)
		}
	}
	if len(r.DestinationJson) > 0 {
		if err := json.Unmarshal(r.DestinationJson, &j.Destination); err != nil {
			return Job{}, fmt.Errorf("unmarshal destination: %w", err)
		}
	}
	if len(r.RetentionJson) > 0 {
		if err := json.Unmarshal(r.RetentionJson, &j.Retention); err != nil {
			return Job{}, fmt.Errorf("unmarshal retention: %w", err)
		}
	}
	return j, nil
}

func rowToRun(r storedb.ReplicationRun) Run {
	run := Run{
		ID:               uuid.UUID(r.ID.Bytes),
		JobID:            uuid.UUID(r.JobID.Bytes),
		Outcome:          RunOutcome(r.Outcome),
		BytesTransferred: r.BytesTransferred,
		Snapshot:         r.Snapshot,
		Error:            r.Error,
	}
	if r.StartedAt.Valid {
		run.StartedAt = r.StartedAt.Time
	}
	if r.FinishedAt.Valid {
		t := r.FinishedAt.Time
		run.FinishedAt = &t
	}
	return run
}
