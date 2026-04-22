package reconciler

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	metapb "github.com/azrtydxb/novanas/storage/api/proto/metadata"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

// fakeMetadataServer is a minimal in-memory MetadataServiceServer used
// for bufconn-based tests. It only implements the RPCs the operators
// StorageClient calls — PutSnapshot / GetSnapshot / DeleteSnapshot and
// PutHealTask / GetHealTask / DeleteHealTask — and embeds
// UnimplementedMetadataServiceServer so the remaining RPCs return
// Unimplemented. Not-found lookups return codes.NotFound to match the
// real server's semantics.
type fakeMetadataServer struct {
	metapb.UnimplementedMetadataServiceServer

	mu        sync.Mutex
	snapshots map[string]*metapb.SnapshotMeta
	tasks     map[string]*metapb.HealTaskMsg

	// putErr / getErr override the normal responses for error-path
	// tests. Reset between subtests.
	putErr error
	getErr error
}

func newFakeServer() *fakeMetadataServer {
	return &fakeMetadataServer{
		snapshots: map[string]*metapb.SnapshotMeta{},
		tasks:     map[string]*metapb.HealTaskMsg{},
	}
}

func (f *fakeMetadataServer) PutSnapshot(_ context.Context, req *metapb.PutSnapshotRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.putErr != nil {
		return nil, f.putErr
	}
	if req.GetMeta() == nil {
		return nil, status.Error(codes.InvalidArgument, "meta required")
	}
	f.snapshots[req.GetMeta().GetSnapshotId()] = req.GetMeta()
	return &emptypb.Empty{}, nil
}

func (f *fakeMetadataServer) GetSnapshot(_ context.Context, req *metapb.GetSnapshotRequest) (*metapb.GetSnapshotResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	m, ok := f.snapshots[req.GetSnapshotId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "snapshot not found")
	}
	return &metapb.GetSnapshotResponse{Meta: m}, nil
}

func (f *fakeMetadataServer) DeleteSnapshot(_ context.Context, req *metapb.DeleteSnapshotRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.snapshots, req.GetSnapshotId())
	return &emptypb.Empty{}, nil
}

func (f *fakeMetadataServer) PutHealTask(_ context.Context, req *metapb.PutHealTaskRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.putErr != nil {
		return nil, f.putErr
	}
	if req.GetTask() == nil {
		return nil, status.Error(codes.InvalidArgument, "task required")
	}
	f.tasks[req.GetTask().GetId()] = req.GetTask()
	return &emptypb.Empty{}, nil
}

func (f *fakeMetadataServer) GetHealTask(_ context.Context, req *metapb.GetHealTaskRequest) (*metapb.GetHealTaskResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	return &metapb.GetHealTaskResponse{Task: t}, nil
}

func (f *fakeMetadataServer) DeleteHealTask(_ context.Context, req *metapb.DeleteHealTaskRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.tasks[req.GetId()]; !ok {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	delete(f.tasks, req.GetId())
	return &emptypb.Empty{}, nil
}

// setTask mutates a stored task (used to simulate engine progress).
func (f *fakeMetadataServer) setTask(id string, t *metapb.HealTaskMsg) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tasks[id] = t
}

// testHarness spins up the fake server on a bufconn listener and
// returns a GRPCStorageClient wired to it plus cleanup.
type testHarness struct {
	Server *fakeMetadataServer
	Client *GRPCStorageClient
	Stop   func()
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	fake := newFakeServer()
	metapb.RegisterMetadataServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()

	dialer := func(_ context.Context, _ string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		srv.Stop()
		t.Fatalf("dial bufconn: %v", err)
	}
	client := NewGRPCStorageClientWithConn(conn, 5*time.Second)

	stop := func() {
		_ = client.Close()
		srv.Stop()
		_ = lis.Close()
	}
	return &testHarness{Server: fake, Client: client, Stop: stop}
}

func TestSnapshotLifecycle(t *testing.T) {
	h := newHarness(t)
	defer h.Stop()

	ctx := context.Background()
	req := SnapshotRequest{VolumeID: "vol-1", SnapshotID: "snap-1", Name: "daily"}

	// Create.
	st, err := h.Client.CreateSnapshot(ctx, req)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if st.Phase != "Completed" {
		t.Fatalf("want Completed, got %q", st.Phase)
	}

	// Get returns ReadyToUse=true → Completed.
	got, err := h.Client.GetSnapshotStatus(ctx, "snap-1")
	if err != nil {
		t.Fatalf("GetSnapshotStatus: %v", err)
	}
	if got.Phase != "Completed" || got.Progress != 100 {
		t.Fatalf("unexpected status: %+v", got)
	}

	// Delete.
	if err := h.Client.DeleteSnapshot(ctx, req); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}

	// Post-delete, Get maps NotFound → assumed completed.
	got, err = h.Client.GetSnapshotStatus(ctx, "snap-1")
	if err != nil {
		t.Fatalf("GetSnapshotStatus after delete: %v", err)
	}
	if got.Phase != "Completed" {
		t.Fatalf("want Completed after delete, got %+v", got)
	}
}

func TestSnapshotMaterializing(t *testing.T) {
	h := newHarness(t)
	defer h.Stop()

	// Pre-populate a snapshot that is not yet ready.
	h.Server.snapshots["snap-prog"] = &metapb.SnapshotMeta{
		SnapshotId:     "snap-prog",
		SourceVolumeId: "vol-1",
		SizeBytes:      1024,
		ReadyToUse:     false,
	}

	got, err := h.Client.GetSnapshotStatus(context.Background(), "snap-prog")
	if err != nil {
		t.Fatalf("GetSnapshotStatus: %v", err)
	}
	if got.Phase != "Running" {
		t.Fatalf("want Running, got %+v", got)
	}
}

func TestReplicationLifecycle(t *testing.T) {
	h := newHarness(t)
	defer h.Stop()

	ctx := context.Background()
	req := ReplicationRequest{JobID: "repl-1", SourceVol: "vol-1", TargetName: "dr-site"}

	st, err := h.Client.StartReplication(ctx, req)
	if err != nil {
		t.Fatalf("StartReplication: %v", err)
	}
	if st.Phase != "Queued" {
		t.Fatalf("want Queued, got %+v", st)
	}

	// Initial get → Queued (pending on server).
	got, err := h.Client.GetReplicationStatus(ctx, "repl-1")
	if err != nil {
		t.Fatalf("GetReplicationStatus: %v", err)
	}
	if got.Phase != "Queued" {
		t.Fatalf("want Queued, got %+v", got)
	}

	// Simulate engine progress: in-progress with 50% done.
	h.Server.setTask("repl-1", &metapb.HealTaskMsg{
		Id:        "repl-1",
		VolumeId:  "vol-1",
		Type:      jobTypeReplication,
		Status:    "in-progress",
		SizeBytes: 1000,
		BytesDone: 500,
	})
	got, err = h.Client.GetReplicationStatus(ctx, "repl-1")
	if err != nil {
		t.Fatalf("GetReplicationStatus: %v", err)
	}
	if got.Phase != "Running" || got.Progress != 50 {
		t.Fatalf("want Running/50, got %+v", got)
	}

	// Simulate completion.
	h.Server.setTask("repl-1", &metapb.HealTaskMsg{
		Id:        "repl-1",
		Type:      jobTypeReplication,
		Status:    "completed",
		SizeBytes: 1000,
		BytesDone: 1000,
	})
	got, err = h.Client.GetReplicationStatus(ctx, "repl-1")
	if err != nil {
		t.Fatalf("GetReplicationStatus: %v", err)
	}
	if got.Phase != "Completed" || got.Progress != 100 {
		t.Fatalf("want Completed/100, got %+v", got)
	}

	// Cancel deletes the task.
	if err := h.Client.CancelReplication(ctx, "repl-1"); err != nil {
		t.Fatalf("CancelReplication: %v", err)
	}
	// Second cancel is idempotent (NotFound tolerated).
	if err := h.Client.CancelReplication(ctx, "repl-1"); err != nil {
		t.Fatalf("CancelReplication idempotent: %v", err)
	}
	// Get after delete returns Completed / unknown-job.
	got, err = h.Client.GetReplicationStatus(ctx, "repl-1")
	if err != nil {
		t.Fatalf("GetReplicationStatus after cancel: %v", err)
	}
	if got.Phase != "Completed" {
		t.Fatalf("want Completed after cancel, got %+v", got)
	}
}

func TestBackupLifecycle(t *testing.T) {
	h := newHarness(t)
	defer h.Stop()

	ctx := context.Background()
	req := BackupRequest{JobID: "bkp-1", VolumeID: "vol-1", Target: "s3://bucket/x"}
	if _, err := h.Client.StartBackup(ctx, req); err != nil {
		t.Fatalf("StartBackup: %v", err)
	}

	// Simulate failure with a message.
	h.Server.setTask("bkp-1", &metapb.HealTaskMsg{
		Id:        "bkp-1",
		Type:      jobTypeBackup,
		Status:    "failed",
		LastError: "s3 credentials rejected",
	})
	got, err := h.Client.GetBackupStatus(ctx, "bkp-1")
	if err != nil {
		t.Fatalf("GetBackupStatus: %v", err)
	}
	if got.Phase != "Failed" || got.Message != "s3 credentials rejected" {
		t.Fatalf("want Failed with message, got %+v", got)
	}
	if err := h.Client.CancelBackup(ctx, "bkp-1"); err != nil {
		t.Fatalf("CancelBackup: %v", err)
	}
}

func TestScrubLifecycle(t *testing.T) {
	h := newHarness(t)
	defer h.Stop()

	ctx := context.Background()
	if _, err := h.Client.StartScrub(ctx, ScrubRequest{Target: "pool-hot"}); err != nil {
		t.Fatalf("StartScrub: %v", err)
	}
	got, err := h.Client.GetScrubStatus(ctx, "pool-hot")
	if err != nil {
		t.Fatalf("GetScrubStatus: %v", err)
	}
	if got.Phase != "Queued" {
		t.Fatalf("want Queued, got %+v", got)
	}
}

func TestErrorMapping_NotFoundOnDelete(t *testing.T) {
	h := newHarness(t)
	defer h.Stop()

	// DeleteSnapshot on a missing ID — the fake returns OK (no-op) just
	// like the real server semantics when the key is already gone. Here
	// we use the non-existent-snapshot GetSnapshot to verify the
	// NotFound → Completed translation.
	got, err := h.Client.GetSnapshotStatus(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Phase != "Completed" {
		t.Fatalf("want Completed for unknown snapshot, got %+v", got)
	}
}

func TestErrorMapping_Unavailable(t *testing.T) {
	h := newHarness(t)
	defer h.Stop()

	h.Server.putErr = status.Error(codes.Unavailable, "busy")

	// StartBackup surfaces Unavailable as ErrServiceUnavailable.
	_, err := h.Client.StartBackup(context.Background(), BackupRequest{JobID: "x", VolumeID: "v"})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("want ErrServiceUnavailable, got %v", err)
	}
}

func TestContextCancellation(t *testing.T) {
	h := newHarness(t)
	defer h.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := h.Client.CreateSnapshot(ctx, SnapshotRequest{VolumeID: "v", SnapshotID: "s"})
	if err == nil {
		t.Fatalf("expected cancellation error, got nil")
	}
	if ctx.Err() == nil {
		t.Fatalf("context should be cancelled")
	}
}
