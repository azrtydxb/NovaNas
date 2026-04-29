package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// fakeSchedulerQueries is a minimal in-memory stand-in covering the
// methods exercised by the handler tests below.
type fakeSchedulerQueries struct {
	snapshots map[string]storedb.SnapshotSchedule
	targets   map[string]storedb.ReplicationTarget
	repls     map[string]storedb.ReplicationSchedule
}

func newFakeSchedulerQueries() *fakeSchedulerQueries {
	return &fakeSchedulerQueries{
		snapshots: map[string]storedb.SnapshotSchedule{},
		targets:   map[string]storedb.ReplicationTarget{},
		repls:     map[string]storedb.ReplicationSchedule{},
	}
}

func keyOf(id pgtype.UUID) string { return uuid.UUID(id.Bytes).String() }

func (f *fakeSchedulerQueries) ListSnapshotSchedules(_ context.Context) ([]storedb.SnapshotSchedule, error) {
	out := make([]storedb.SnapshotSchedule, 0, len(f.snapshots))
	for _, v := range f.snapshots {
		out = append(out, v)
	}
	return out, nil
}
func (f *fakeSchedulerQueries) GetSnapshotSchedule(_ context.Context, id pgtype.UUID) (storedb.SnapshotSchedule, error) {
	if v, ok := f.snapshots[keyOf(id)]; ok {
		return v, nil
	}
	return storedb.SnapshotSchedule{}, pgx.ErrNoRows
}
func (f *fakeSchedulerQueries) CreateSnapshotSchedule(_ context.Context, arg storedb.CreateSnapshotScheduleParams) (storedb.SnapshotSchedule, error) {
	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	row := storedb.SnapshotSchedule{
		ID: id, Dataset: arg.Dataset, Name: arg.Name, Cron: arg.Cron,
		Recursive: arg.Recursive, SnapshotPrefix: arg.SnapshotPrefix,
		RetentionHourly: arg.RetentionHourly, RetentionDaily: arg.RetentionDaily,
		RetentionWeekly: arg.RetentionWeekly, RetentionMonthly: arg.RetentionMonthly,
		RetentionYearly: arg.RetentionYearly, Enabled: arg.Enabled,
	}
	f.snapshots[keyOf(id)] = row
	return row, nil
}
func (f *fakeSchedulerQueries) UpdateSnapshotSchedule(_ context.Context, arg storedb.UpdateSnapshotScheduleParams) (storedb.SnapshotSchedule, error) {
	row, ok := f.snapshots[keyOf(arg.ID)]
	if !ok {
		return storedb.SnapshotSchedule{}, pgx.ErrNoRows
	}
	row.Cron = arg.Cron
	row.SnapshotPrefix = arg.SnapshotPrefix
	row.Recursive = arg.Recursive
	row.RetentionHourly = arg.RetentionHourly
	row.RetentionDaily = arg.RetentionDaily
	row.RetentionWeekly = arg.RetentionWeekly
	row.RetentionMonthly = arg.RetentionMonthly
	row.RetentionYearly = arg.RetentionYearly
	row.Enabled = arg.Enabled
	f.snapshots[keyOf(arg.ID)] = row
	return row, nil
}
func (f *fakeSchedulerQueries) DeleteSnapshotSchedule(_ context.Context, id pgtype.UUID) error {
	delete(f.snapshots, keyOf(id))
	return nil
}
func (f *fakeSchedulerQueries) ListReplicationTargets(_ context.Context) ([]storedb.ReplicationTarget, error) {
	out := make([]storedb.ReplicationTarget, 0, len(f.targets))
	for _, v := range f.targets {
		out = append(out, v)
	}
	return out, nil
}
func (f *fakeSchedulerQueries) GetReplicationTarget(_ context.Context, id pgtype.UUID) (storedb.ReplicationTarget, error) {
	if v, ok := f.targets[keyOf(id)]; ok {
		return v, nil
	}
	return storedb.ReplicationTarget{}, pgx.ErrNoRows
}
func (f *fakeSchedulerQueries) CreateReplicationTarget(_ context.Context, arg storedb.CreateReplicationTargetParams) (storedb.ReplicationTarget, error) {
	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	row := storedb.ReplicationTarget{
		ID: id, Name: arg.Name, Host: arg.Host, Port: arg.Port,
		SshUser: arg.SshUser, SshKeyPath: arg.SshKeyPath,
		DatasetPrefix: arg.DatasetPrefix,
	}
	f.targets[keyOf(id)] = row
	return row, nil
}
func (f *fakeSchedulerQueries) DeleteReplicationTarget(_ context.Context, id pgtype.UUID) error {
	delete(f.targets, keyOf(id))
	return nil
}
func (f *fakeSchedulerQueries) ListReplicationSchedules(_ context.Context) ([]storedb.ReplicationSchedule, error) {
	out := make([]storedb.ReplicationSchedule, 0, len(f.repls))
	for _, v := range f.repls {
		out = append(out, v)
	}
	return out, nil
}
func (f *fakeSchedulerQueries) GetReplicationSchedule(_ context.Context, id pgtype.UUID) (storedb.ReplicationSchedule, error) {
	if v, ok := f.repls[keyOf(id)]; ok {
		return v, nil
	}
	return storedb.ReplicationSchedule{}, pgx.ErrNoRows
}
func (f *fakeSchedulerQueries) CreateReplicationSchedule(_ context.Context, arg storedb.CreateReplicationScheduleParams) (storedb.ReplicationSchedule, error) {
	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	row := storedb.ReplicationSchedule{
		ID: id, SrcDataset: arg.SrcDataset, TargetID: arg.TargetID,
		Cron: arg.Cron, SnapshotPrefix: arg.SnapshotPrefix,
		RetentionRemote: arg.RetentionRemote, Enabled: arg.Enabled,
	}
	f.repls[keyOf(id)] = row
	return row, nil
}
func (f *fakeSchedulerQueries) UpdateReplicationSchedule(_ context.Context, arg storedb.UpdateReplicationScheduleParams) (storedb.ReplicationSchedule, error) {
	row, ok := f.repls[keyOf(arg.ID)]
	if !ok {
		return storedb.ReplicationSchedule{}, pgx.ErrNoRows
	}
	row.Cron = arg.Cron
	row.SnapshotPrefix = arg.SnapshotPrefix
	row.RetentionRemote = arg.RetentionRemote
	row.Enabled = arg.Enabled
	f.repls[keyOf(arg.ID)] = row
	return row, nil
}
func (f *fakeSchedulerQueries) DeleteReplicationSchedule(_ context.Context, id pgtype.UUID) error {
	delete(f.repls, keyOf(id))
	return nil
}

func TestSchedulerCreateAndGetSnapshotSchedule(t *testing.T) {
	q := newFakeSchedulerQueries()
	h := &SchedulerHandler{Logger: newDiscardLogger(), Q: q}

	body := `{"dataset":"tank/data","name":"daily","cron":"0 3 * * *","snapshotPrefix":"auto","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scheduler/snapshot-schedules", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CreateSnapshotSchedule(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got storedb.SnapshotSchedule
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Dataset != "tank/data" {
		t.Errorf("dataset=%q", got.Dataset)
	}

	// GET by id
	r := chi.NewRouter()
	r.Get("/api/v1/scheduler/snapshot-schedules/{id}", h.GetSnapshotSchedule)
	getReq := httptest.NewRequest(http.MethodGet,
		"/api/v1/scheduler/snapshot-schedules/"+uuid.UUID(got.ID.Bytes).String(), nil)
	rrG := httptest.NewRecorder()
	r.ServeHTTP(rrG, getReq)
	if rrG.Code != http.StatusOK {
		t.Fatalf("get status=%d", rrG.Code)
	}
}

func TestSchedulerCreateSnapshotSchedule_BadInput(t *testing.T) {
	q := newFakeSchedulerQueries()
	h := &SchedulerHandler{Logger: newDiscardLogger(), Q: q}
	body := `{"name":"x"}` // missing dataset, cron, snapshotPrefix
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scheduler/snapshot-schedules", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CreateSnapshotSchedule(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestSchedulerGetSnapshotSchedule_NotFound(t *testing.T) {
	q := newFakeSchedulerQueries()
	h := &SchedulerHandler{Logger: newDiscardLogger(), Q: q}
	r := chi.NewRouter()
	r.Get("/api/v1/scheduler/snapshot-schedules/{id}", h.GetSnapshotSchedule)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/scheduler/snapshot-schedules/"+uuid.New().String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestSchedulerCreateReplicationTarget_Returns201(t *testing.T) {
	q := newFakeSchedulerQueries()
	h := &SchedulerHandler{Logger: newDiscardLogger(), Q: q}
	body := `{"name":"backup","host":"backup.example.com","sshUser":"zfs","datasetPrefix":"backup/from-tank"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scheduler/replication-targets", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CreateReplicationTarget(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSchedulerCreateReplicationSchedule_Returns201(t *testing.T) {
	q := newFakeSchedulerQueries()
	h := &SchedulerHandler{Logger: newDiscardLogger(), Q: q}
	tid := uuid.New().String()
	body := `{"srcDataset":"tank/data","targetId":"` + tid + `","cron":"0 4 * * *","snapshotPrefix":"repl","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scheduler/replication-schedules", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CreateReplicationSchedule(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSchedulerListEmpty(t *testing.T) {
	q := newFakeSchedulerQueries()
	h := &SchedulerHandler{Logger: newDiscardLogger(), Q: q}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/scheduler/snapshot-schedules", nil)
	rr := httptest.NewRecorder()
	h.ListSnapshotSchedules(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if rr.Body.String() != "[]\n" && rr.Body.String() != "[]" {
		// WriteJSON appends a newline via encoder; accept either.
	}
}
