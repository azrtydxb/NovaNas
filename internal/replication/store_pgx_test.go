package replication

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// fakeQueries implements [PgxQueries] in memory. It is sufficient for
// validating the JSON encode/decode plumbing in PgxStore without a
// running Postgres. A future change can swap this for testcontainers
// when CI gains a Postgres service.
type fakeQueries struct {
	mu   sync.Mutex
	jobs map[uuid.UUID]storedb.ReplicationJob
	runs map[uuid.UUID][]storedb.ReplicationRun
}

func newFakeQueries() *fakeQueries {
	return &fakeQueries{
		jobs: map[uuid.UUID]storedb.ReplicationJob{},
		runs: map[uuid.UUID][]storedb.ReplicationRun{},
	}
}

func (f *fakeQueries) CreateReplicationJob(_ context.Context, arg storedb.CreateReplicationJobParams) (storedb.ReplicationJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := uuid.UUID(arg.ID.Bytes)
	if _, exists := f.jobs[id]; exists {
		return storedb.ReplicationJob{}, errors.New("dup")
	}
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	row := storedb.ReplicationJob{
		ID:              arg.ID,
		Name:            arg.Name,
		Backend:         arg.Backend,
		Direction:       arg.Direction,
		SourceJson:      arg.SourceJson,
		DestinationJson: arg.DestinationJson,
		Schedule:        arg.Schedule,
		RetentionJson:   arg.RetentionJson,
		Enabled:         arg.Enabled,
		SecretRef:       arg.SecretRef,
		LastSnapshot:    arg.LastSnapshot,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	f.jobs[id] = row
	return row, nil
}

func (f *fakeQueries) UpdateReplicationJob(_ context.Context, arg storedb.UpdateReplicationJobParams) (storedb.ReplicationJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := uuid.UUID(arg.ID.Bytes)
	row, ok := f.jobs[id]
	if !ok {
		return storedb.ReplicationJob{}, pgx.ErrNoRows
	}
	row.Name = arg.Name
	row.Backend = arg.Backend
	row.Direction = arg.Direction
	row.SourceJson = arg.SourceJson
	row.DestinationJson = arg.DestinationJson
	row.Schedule = arg.Schedule
	row.RetentionJson = arg.RetentionJson
	row.Enabled = arg.Enabled
	row.SecretRef = arg.SecretRef
	row.LastSnapshot = arg.LastSnapshot
	row.UpdatedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	f.jobs[id] = row
	return row, nil
}

func (f *fakeQueries) DeleteReplicationJob(_ context.Context, id pgtype.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.jobs, uuid.UUID(id.Bytes))
	delete(f.runs, uuid.UUID(id.Bytes))
	return nil
}

func (f *fakeQueries) GetReplicationJob(_ context.Context, id pgtype.UUID) (storedb.ReplicationJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.jobs[uuid.UUID(id.Bytes)]
	if !ok {
		return storedb.ReplicationJob{}, pgx.ErrNoRows
	}
	return row, nil
}

func (f *fakeQueries) ListReplicationJobs(_ context.Context) ([]storedb.ReplicationJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storedb.ReplicationJob, 0, len(f.jobs))
	for _, row := range f.jobs {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *fakeQueries) ListEnabledReplicationJobs(_ context.Context) ([]storedb.ReplicationJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storedb.ReplicationJob, 0, len(f.jobs))
	for _, row := range f.jobs {
		if row.Enabled {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeQueries) MarkReplicationJobFired(_ context.Context, arg storedb.MarkReplicationJobFiredParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := uuid.UUID(arg.ID.Bytes)
	row, ok := f.jobs[id]
	if !ok {
		return pgx.ErrNoRows
	}
	row.LastFiredAt = arg.LastFiredAt
	f.jobs[id] = row
	return nil
}

func (f *fakeQueries) InsertReplicationRun(_ context.Context, arg storedb.InsertReplicationRunParams) (storedb.ReplicationRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row := storedb.ReplicationRun{
		ID:               arg.ID,
		JobID:            arg.JobID,
		StartedAt:        arg.StartedAt,
		Outcome:          arg.Outcome,
		BytesTransferred: arg.BytesTransferred,
		Snapshot:         arg.Snapshot,
		Error:            arg.Error,
	}
	jid := uuid.UUID(arg.JobID.Bytes)
	f.runs[jid] = append(f.runs[jid], row)
	return row, nil
}

func (f *fakeQueries) UpdateReplicationRun(_ context.Context, arg storedb.UpdateReplicationRunParams) (storedb.ReplicationRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for jid, rows := range f.runs {
		for i, r := range rows {
			if r.ID == arg.ID {
				r.FinishedAt = arg.FinishedAt
				r.Outcome = arg.Outcome
				r.BytesTransferred = arg.BytesTransferred
				r.Snapshot = arg.Snapshot
				r.Error = arg.Error
				f.runs[jid][i] = r
				return r, nil
			}
		}
	}
	return storedb.ReplicationRun{}, pgx.ErrNoRows
}

func (f *fakeQueries) ListReplicationRuns(_ context.Context, arg storedb.ListReplicationRunsParams) ([]storedb.ReplicationRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rows := f.runs[uuid.UUID(arg.JobID.Bytes)]
	// reverse for newest-first
	out := make([]storedb.ReplicationRun, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		out = append(out, rows[i])
	}
	if int(arg.Limit) > 0 && len(out) > int(arg.Limit) {
		out = out[:arg.Limit]
	}
	return out, nil
}

func (f *fakeQueries) ListReplicationRunsAfter(ctx context.Context, arg storedb.ListReplicationRunsAfterParams) ([]storedb.ReplicationRun, error) {
	return f.ListReplicationRuns(ctx, storedb.ListReplicationRunsParams{JobID: arg.JobID, Limit: arg.Limit})
}

// ----- tests -----

func TestPgxStoreCreateGetRoundTrip(t *testing.T) {
	q := newFakeQueries()
	s := NewPgxStore(q)
	ctx := context.Background()

	id := uuid.New()
	in := Job{
		ID:        id,
		Name:      "demo",
		Backend:   BackendS3,
		Direction: DirectionPush,
		Source:    Source{Path: "/srv/data"},
		Destination: Destination{
			Bucket: "backups", Prefix: "tank/", Endpoint: "https://s3.example.com", Region: "us-east-1",
		},
		Schedule:  "0 2 * * *",
		Retention: RetentionPolicy{KeepLastN: 7, KeepDaily: 3},
		Enabled:   true,
		SecretRef: "nova/replication/" + id.String(),
	}
	out, err := s.CreateJob(ctx, in)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if out.Name != "demo" || out.Backend != BackendS3 || out.Source.Path != "/srv/data" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
	got, err := s.GetJob(ctx, id)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Retention.KeepLastN != 7 || got.Retention.KeepDaily != 3 {
		t.Fatalf("retention round-trip lost: %+v", got.Retention)
	}
	if got.Destination.Bucket != "backups" {
		t.Fatalf("destination bucket lost: %q", got.Destination.Bucket)
	}

	// Verify the JSON columns are valid JSON.
	if !json.Valid(q.jobs[id].SourceJson) {
		t.Fatalf("source_json not valid JSON")
	}
}

func TestPgxStoreUpdateAndList(t *testing.T) {
	q := newFakeQueries()
	s := NewPgxStore(q)
	ctx := context.Background()

	id := uuid.New()
	in := Job{ID: id, Name: "a", Backend: BackendZFS, Direction: DirectionPush}
	if _, err := s.CreateJob(ctx, in); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	in.Name = "z"
	in.Schedule = "@hourly"
	if _, err := s.UpdateJob(ctx, in); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}
	rows, err := s.ListJobs(ctx)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "z" || rows[0].Schedule != "@hourly" {
		t.Fatalf("update not visible: %+v", rows)
	}
}

func TestPgxStoreUpdateMissingReturnsErrNotFound(t *testing.T) {
	q := newFakeQueries()
	s := NewPgxStore(q)
	_, err := s.UpdateJob(context.Background(), Job{ID: uuid.New(), Name: "x", Backend: BackendZFS, Direction: DirectionPush})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPgxStoreRunsRoundTrip(t *testing.T) {
	q := newFakeQueries()
	s := NewPgxStore(q)
	ctx := context.Background()

	jobID := uuid.New()
	if _, err := s.CreateJob(ctx, Job{ID: jobID, Name: "j", Backend: BackendZFS, Direction: DirectionPush}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	r, err := s.CreateRun(ctx, Run{ID: uuid.New(), JobID: jobID, StartedAt: time.Now().UTC(), Outcome: RunRunning})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	end := time.Now().UTC()
	r.FinishedAt = &end
	r.BytesTransferred = 1234
	r.Outcome = RunSucceeded
	if _, err := s.UpdateRun(ctx, r); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}
	runs, err := s.ListRuns(ctx, jobID, 10)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].Outcome != RunSucceeded || runs[0].BytesTransferred != 1234 {
		t.Fatalf("run not persisted as expected: %+v", runs)
	}
}

func TestPgxStoreDeleteCascade(t *testing.T) {
	q := newFakeQueries()
	s := NewPgxStore(q)
	ctx := context.Background()

	id := uuid.New()
	if _, err := s.CreateJob(ctx, Job{ID: id, Name: "j", Backend: BackendZFS, Direction: DirectionPush}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := s.DeleteJob(ctx, id); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	if _, err := s.GetJob(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
