package reconciler

import "context"

// SnapshotRequest captures the minimal payload the storage engine needs to
// create or destroy a point-in-time snapshot.
type SnapshotRequest struct {
	VolumeID   string
	SnapshotID string
	Name       string
}

// ReplicationRequest captures parameters for an async replication operation
// between a source volume and a remote target.
type ReplicationRequest struct {
	JobID      string
	SourceVol  string
	TargetURL  string
	TargetName string
	Incremental bool
}

// BackupRequest captures cloud-backup parameters.
type BackupRequest struct {
	JobID    string
	VolumeID string
	Target   string // e.g. s3://bucket/path
}

// ScrubRequest asks the engine to scrub a pool or volume.
type ScrubRequest struct {
	Target string
}

// StorageOpStatus is returned by long-running storage operations so the
// reconciler can surface progress in status.
type StorageOpStatus struct {
	Phase       string // Queued | Running | Completed | Failed
	Progress    int32  // 0..100
	BytesTotal  int64
	BytesDone   int64
	Message     string
}

// StorageClient is the adapter controllers call into for operations that
// must be executed by the storage data-plane. Real implementations wrap
// the gRPC clients against the NovaFlow storage engine; tests use
// NoopStorageClient.
type StorageClient interface {
	CreateSnapshot(ctx context.Context, req SnapshotRequest) (StorageOpStatus, error)
	DeleteSnapshot(ctx context.Context, req SnapshotRequest) error
	GetSnapshotStatus(ctx context.Context, snapshotID string) (StorageOpStatus, error)

	StartReplication(ctx context.Context, req ReplicationRequest) (StorageOpStatus, error)
	GetReplicationStatus(ctx context.Context, jobID string) (StorageOpStatus, error)
	CancelReplication(ctx context.Context, jobID string) error

	StartBackup(ctx context.Context, req BackupRequest) (StorageOpStatus, error)
	GetBackupStatus(ctx context.Context, jobID string) (StorageOpStatus, error)
	CancelBackup(ctx context.Context, jobID string) error

	StartScrub(ctx context.Context, req ScrubRequest) (StorageOpStatus, error)
	GetScrubStatus(ctx context.Context, target string) (StorageOpStatus, error)
}

// NoopStorageClient is the default fallback. Every operation reports a
// "Completed" status with zero progress so reconcilers can move through
// their state machine in dev/test without a real storage engine attached.
type NoopStorageClient struct{}

func noopCompleted(msg string) StorageOpStatus {
	return StorageOpStatus{Phase: "Completed", Progress: 100, Message: msg}
}

// CreateSnapshot is a no-op that returns Completed.
func (NoopStorageClient) CreateSnapshot(_ context.Context, _ SnapshotRequest) (StorageOpStatus, error) {
	return noopCompleted("noop snapshot created"), nil
}

// DeleteSnapshot is a no-op.
func (NoopStorageClient) DeleteSnapshot(_ context.Context, _ SnapshotRequest) error { return nil }

// GetSnapshotStatus returns Completed.
func (NoopStorageClient) GetSnapshotStatus(_ context.Context, _ string) (StorageOpStatus, error) {
	return noopCompleted("noop"), nil
}

// StartReplication returns Completed.
func (NoopStorageClient) StartReplication(_ context.Context, _ ReplicationRequest) (StorageOpStatus, error) {
	return noopCompleted("noop replication started"), nil
}

// GetReplicationStatus returns Completed.
func (NoopStorageClient) GetReplicationStatus(_ context.Context, _ string) (StorageOpStatus, error) {
	return noopCompleted("noop"), nil
}

// CancelReplication is a no-op.
func (NoopStorageClient) CancelReplication(_ context.Context, _ string) error { return nil }

// StartBackup returns Completed.
func (NoopStorageClient) StartBackup(_ context.Context, _ BackupRequest) (StorageOpStatus, error) {
	return noopCompleted("noop backup started"), nil
}

// GetBackupStatus returns Completed.
func (NoopStorageClient) GetBackupStatus(_ context.Context, _ string) (StorageOpStatus, error) {
	return noopCompleted("noop"), nil
}

// CancelBackup is a no-op.
func (NoopStorageClient) CancelBackup(_ context.Context, _ string) error { return nil }

// StartScrub returns Completed.
func (NoopStorageClient) StartScrub(_ context.Context, _ ScrubRequest) (StorageOpStatus, error) {
	return noopCompleted("noop scrub started"), nil
}

// GetScrubStatus returns Completed.
func (NoopStorageClient) GetScrubStatus(_ context.Context, _ string) (StorageOpStatus, error) {
	return noopCompleted("noop"), nil
}
