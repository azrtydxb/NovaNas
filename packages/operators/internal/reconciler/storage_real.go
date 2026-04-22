// Package reconciler — typed gRPC-backed implementation of StorageClient.
//
// GRPCStorageClient wraps a metadata.MetadataServiceClient (generated from
// storage/api/proto/metadata/metadata.proto) and translates controller
// StorageClient calls into real gRPC RPCs against the NovaNas storage
// metadata service. There is no in-memory job table: every operation
// round-trips to the metadata service.
//
// Mapping of the StorageClient surface onto the metadata proto:
//
//   - CreateSnapshot / DeleteSnapshot / GetSnapshotStatus → PutSnapshot /
//     DeleteSnapshot / GetSnapshot. The SnapshotMeta.ReadyToUse field is
//     the terminal-state signal surfaced as Phase=Completed.
//
//   - StartReplication / GetReplicationStatus / CancelReplication,
//     StartBackup / GetBackupStatus / CancelBackup, StartScrub /
//     GetScrubStatus → PutHealTask / GetHealTask / DeleteHealTask.
//     HealTask is the existing "background storage job" record in the
//     metadata store (see storage/internal/metadata/protection_meta.go).
//     Its ID/Type/Status/BytesDone/SizeBytes/LastError fields map
//     cleanly to StorageOpStatus. The Type field is tagged with one of
//     "replication", "backup", "scrub" so the engine side can dispatch.
//
// The connection is created via grpc.NewClient (the non-deprecated
// constructor), with TLS negotiated from env-driven PEM file paths or
// plaintext insecure credentials for in-cluster dev use. The package
// exposes Close() so manager shutdown can release the conn cleanly.
package reconciler

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	metapb "github.com/azrtydxb/novanas/storage/api/proto/metadata"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Typed errors returned from gRPC calls so callers can pattern-match
// without importing google.golang.org/grpc/status.
var (
	// ErrNotFound wraps codes.NotFound responses from the metadata service.
	ErrNotFound = errors.New("storage: not found")
	// ErrAlreadyExists wraps codes.AlreadyExists responses.
	ErrAlreadyExists = errors.New("storage: already exists")
	// ErrServiceUnavailable wraps codes.Unavailable after retries are
	// exhausted. The transient error surface is controlled by the
	// gRPC service-config retry policy in NewGRPCStorageClient.
	ErrServiceUnavailable = errors.New("storage: service unavailable")
)

// Job type tags persisted in HealTask.Type so the engine and the
// operator agree on dispatch.
const (
	jobTypeReplication = "replication"
	jobTypeBackup      = "backup"
	jobTypeScrub       = "scrub"
)

// GRPCStorageClient is a StorageClient backed by a real gRPC connection
// to the NovaNas storage metadata service. Every method maps onto a
// generated MetadataServiceClient RPC.
type GRPCStorageClient struct {
	addr   string
	conn   *grpc.ClientConn
	client metapb.MetadataServiceClient
	callTO time.Duration
}

// GRPCStorageClientConfig tunes dial behaviour.
type GRPCStorageClientConfig struct {
	Address       string
	CAFile        string
	CertFile      string
	KeyFile       string
	ServerName    string
	DialTimeout   time.Duration
	CallTimeout   time.Duration
	ExtraDialOpts []grpc.DialOption
}

// retryServiceConfig mirrors the storage GRPCClient's retry policy so
// UNAVAILABLE replies retry automatically.
const retryServiceConfig = `{
	"methodConfig": [{
		"name": [{"service": "metadata.MetadataService"}],
		"retryPolicy": {
			"maxAttempts": 5,
			"initialBackoff": "0.1s",
			"maxBackoff": "1s",
			"backoffMultiplier": 2.0,
			"retryableStatusCodes": ["UNAVAILABLE"]
		}
	}]
}`

// NewGRPCStorageClient dials the metadata service using grpc.NewClient
// (non-deprecated) and returns a ready-to-use client. When all three
// TLS file paths are provided, the connection uses mTLS; otherwise it
// falls back to plaintext (suitable for in-cluster service meshes that
// terminate TLS themselves).
func NewGRPCStorageClient(cfg GRPCStorageClientConfig) (*GRPCStorageClient, error) {
	if cfg.Address == "" {
		return nil, errors.New("storage client: Address is required")
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 10 * time.Second
	}
	if cfg.CallTimeout == 0 {
		cfg.CallTimeout = 10 * time.Second
	}

	opts := append([]grpc.DialOption{}, cfg.ExtraDialOpts...)
	if cfg.CAFile != "" && cfg.CertFile != "" && cfg.KeyFile != "" {
		tlsCfg, err := loadMTLS(cfg.CAFile, cfg.CertFile, cfg.KeyFile, cfg.ServerName)
		if err != nil {
			return nil, fmt.Errorf("storage client: load mTLS: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	opts = append(opts, grpc.WithDefaultServiceConfig(retryServiceConfig))

	// grpc.NewClient replaces the deprecated grpc.DialContext. It is
	// non-blocking; the first RPC transparently waits for the
	// underlying transport to be ready, and reconnection on
	// disconnect is handled by gRPC transparently.
	target := cfg.Address
	if !strings.Contains(target, "://") {
		target = "dns:///" + target
	}
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("storage client: dial %s: %w", cfg.Address, err)
	}
	return &GRPCStorageClient{
		addr:   cfg.Address,
		conn:   conn,
		client: metapb.NewMetadataServiceClient(conn),
		callTO: cfg.CallTimeout,
	}, nil
}

// NewGRPCStorageClientWithConn wraps an existing gRPC connection. Used
// by tests that dial a bufconn listener rather than a real address.
func NewGRPCStorageClientWithConn(conn *grpc.ClientConn, callTimeout time.Duration) *GRPCStorageClient {
	if callTimeout == 0 {
		callTimeout = 10 * time.Second
	}
	return &GRPCStorageClient{
		addr:   "in-process",
		conn:   conn,
		client: metapb.NewMetadataServiceClient(conn),
		callTO: callTimeout,
	}
}

// Close releases the gRPC connection.
func (c *GRPCStorageClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Address returns the configured server address.
func (c *GRPCStorageClient) Address() string { return c.addr }

func (c *GRPCStorageClient) withCallTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.callTO == 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.callTO)
}

// mapErr converts gRPC status codes into typed sentinels so callers can
// branch on errors.Is instead of reaching into grpc/status.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	switch st.Code() {
	case codes.NotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, st.Message())
	case codes.AlreadyExists:
		return fmt.Errorf("%w: %s", ErrAlreadyExists, st.Message())
	case codes.Unavailable:
		return fmt.Errorf("%w: %s", ErrServiceUnavailable, st.Message())
	case codes.Canceled, codes.DeadlineExceeded:
		return err
	default:
		return err
	}
}

// ---- Snapshot operations (real PutSnapshot / GetSnapshot / DeleteSnapshot) ----

// CreateSnapshot persists a SnapshotMeta via PutSnapshot. The resulting
// StorageOpStatus reports Phase=Completed once the metadata record is
// durable in the store.
func (c *GRPCStorageClient) CreateSnapshot(ctx context.Context, req SnapshotRequest) (StorageOpStatus, error) {
	ctx, cancel := c.withCallTimeout(ctx)
	defer cancel()
	meta := &metapb.SnapshotMeta{
		SnapshotId:     req.SnapshotID,
		SourceVolumeId: req.VolumeID,
		CreationTime:   time.Now().UnixNano(),
		ReadyToUse:     true,
	}
	if _, err := c.client.PutSnapshot(ctx, &metapb.PutSnapshotRequest{Meta: meta}); err != nil {
		return StorageOpStatus{Phase: "Failed", Message: err.Error()}, mapErr(err)
	}
	return StorageOpStatus{Phase: "Completed", Progress: 100, Message: "snapshot created: " + req.Name}, nil
}

// DeleteSnapshot removes the SnapshotMeta via DeleteSnapshot.
func (c *GRPCStorageClient) DeleteSnapshot(ctx context.Context, req SnapshotRequest) error {
	ctx, cancel := c.withCallTimeout(ctx)
	defer cancel()
	_, err := c.client.DeleteSnapshot(ctx, &metapb.DeleteSnapshotRequest{SnapshotId: req.SnapshotID})
	return mapErr(err)
}

// GetSnapshotStatus fetches the SnapshotMeta and converts ReadyToUse
// into a StorageOpStatus. A NotFound response is surfaced as a
// deterministic "unknown / assumed completed" status so reconcilers
// stay idempotent across deletions.
func (c *GRPCStorageClient) GetSnapshotStatus(ctx context.Context, snapshotID string) (StorageOpStatus, error) {
	ctx, cancel := c.withCallTimeout(ctx)
	defer cancel()
	resp, err := c.client.GetSnapshot(ctx, &metapb.GetSnapshotRequest{SnapshotId: snapshotID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return StorageOpStatus{Phase: "Completed", Progress: 100, Message: "snapshot not found; assumed deleted"}, nil
		}
		return StorageOpStatus{Phase: "Failed", Message: err.Error()}, mapErr(err)
	}
	meta := resp.GetMeta()
	if meta == nil {
		return StorageOpStatus{Phase: "Completed", Progress: 100, Message: "empty snapshot meta"}, nil
	}
	st := StorageOpStatus{
		BytesTotal: int64(meta.GetSizeBytes()),
		BytesDone:  int64(meta.GetSizeBytes()),
	}
	if meta.GetReadyToUse() {
		st.Phase = "Completed"
		st.Progress = 100
		st.Message = "snapshot ready"
	} else {
		st.Phase = "Running"
		st.Progress = 0
		st.Message = "snapshot materializing"
	}
	return st, nil
}

// ---- Replication / Backup / Scrub — backed by HealTask RPCs. ----
//
// HealTask is the metadata store's existing "background storage job"
// record. We reuse it for replication/backup/scrub by tagging the
// HealTask.Type field with one of jobTypeReplication / jobTypeBackup /
// jobTypeScrub so the engine side can dispatch them. The {ID, Status,
// BytesDone, SizeBytes, LastError} fields surface directly as
// StorageOpStatus.

// StartReplication queues a replication HealTask via PutHealTask.
func (c *GRPCStorageClient) StartReplication(ctx context.Context, req ReplicationRequest) (StorageOpStatus, error) {
	return c.startJob(ctx, jobTypeReplication, req.JobID, req.SourceVol, "replication accepted: "+req.TargetName)
}

// GetReplicationStatus looks up the HealTask and converts status fields
// into StorageOpStatus.
func (c *GRPCStorageClient) GetReplicationStatus(ctx context.Context, jobID string) (StorageOpStatus, error) {
	return c.getJob(ctx, jobID)
}

// CancelReplication deletes the HealTask record.
func (c *GRPCStorageClient) CancelReplication(ctx context.Context, jobID string) error {
	return c.cancelJob(ctx, jobID)
}

// StartBackup queues a backup HealTask.
func (c *GRPCStorageClient) StartBackup(ctx context.Context, req BackupRequest) (StorageOpStatus, error) {
	return c.startJob(ctx, jobTypeBackup, req.JobID, req.VolumeID, "backup accepted: "+req.Target)
}

// GetBackupStatus looks up the backup HealTask.
func (c *GRPCStorageClient) GetBackupStatus(ctx context.Context, jobID string) (StorageOpStatus, error) {
	return c.getJob(ctx, jobID)
}

// CancelBackup deletes the HealTask record.
func (c *GRPCStorageClient) CancelBackup(ctx context.Context, jobID string) error {
	return c.cancelJob(ctx, jobID)
}

// StartScrub queues a scrub HealTask keyed by the target pool name.
func (c *GRPCStorageClient) StartScrub(ctx context.Context, req ScrubRequest) (StorageOpStatus, error) {
	id := "scrub:" + req.Target
	return c.startJob(ctx, jobTypeScrub, id, req.Target, "scrub accepted: "+req.Target)
}

// GetScrubStatus looks up the scrub HealTask.
func (c *GRPCStorageClient) GetScrubStatus(ctx context.Context, target string) (StorageOpStatus, error) {
	return c.getJob(ctx, "scrub:"+target)
}

// startJob persists a HealTask with the given type and Status="pending".
func (c *GRPCStorageClient) startJob(ctx context.Context, jobType, id, volumeID, message string) (StorageOpStatus, error) {
	ctx, cancel := c.withCallTimeout(ctx)
	defer cancel()
	now := time.Now().Unix()
	task := &metapb.HealTaskMsg{
		Id:        id,
		VolumeId:  volumeID,
		Type:      jobType,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := c.client.PutHealTask(ctx, &metapb.PutHealTaskRequest{Task: task}); err != nil {
		return StorageOpStatus{Phase: "Failed", Message: err.Error()}, mapErr(err)
	}
	return StorageOpStatus{Phase: "Queued", Progress: 0, Message: message}, nil
}

// getJob fetches a HealTask and converts it to StorageOpStatus. Unknown
// jobs map to a deterministic "assumed completed" status so the
// reconcile loop doesn't block after a delete.
func (c *GRPCStorageClient) getJob(ctx context.Context, id string) (StorageOpStatus, error) {
	ctx, cancel := c.withCallTimeout(ctx)
	defer cancel()
	resp, err := c.client.GetHealTask(ctx, &metapb.GetHealTaskRequest{Id: id})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return StorageOpStatus{Phase: "Completed", Progress: 100, Message: "unknown job; assumed completed"}, nil
		}
		return StorageOpStatus{Phase: "Failed", Message: err.Error()}, mapErr(err)
	}
	return healTaskToOpStatus(resp.GetTask()), nil
}

// cancelJob deletes the HealTask record. NotFound is tolerated as a
// no-op so Cancel is idempotent.
func (c *GRPCStorageClient) cancelJob(ctx context.Context, id string) error {
	ctx, cancel := c.withCallTimeout(ctx)
	defer cancel()
	_, err := c.client.DeleteHealTask(ctx, &metapb.DeleteHealTaskRequest{Id: id})
	if err != nil && status.Code(err) == codes.NotFound {
		return nil
	}
	return mapErr(err)
}

// healTaskToOpStatus converts HealTaskMsg.Status into the reconciler's
// Phase vocabulary and surfaces BytesDone/SizeBytes for progress.
func healTaskToOpStatus(t *metapb.HealTaskMsg) StorageOpStatus {
	if t == nil {
		return StorageOpStatus{Phase: "Completed", Progress: 100, Message: "empty task"}
	}
	op := StorageOpStatus{
		BytesTotal: t.GetSizeBytes(),
		BytesDone:  t.GetBytesDone(),
		Message:    t.GetLastError(),
	}
	if t.GetSizeBytes() > 0 {
		op.Progress = int32((t.GetBytesDone() * 100) / t.GetSizeBytes())
		if op.Progress > 100 {
			op.Progress = 100
		}
	}
	switch t.GetStatus() {
	case "completed":
		op.Phase = "Completed"
		op.Progress = 100
		if op.Message == "" {
			op.Message = "job completed"
		}
	case "failed":
		op.Phase = "Failed"
		if op.Message == "" {
			op.Message = "job failed"
		}
	case "in-progress":
		op.Phase = "Running"
		if op.Message == "" {
			op.Message = "job running"
		}
	default: // "pending" and unknown
		op.Phase = "Queued"
		if op.Message == "" {
			op.Message = "job queued"
		}
	}
	return op
}

func loadMTLS(caFile, certFile, keyFile, serverName string) (*tls.Config, error) {
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("parse CA PEM: no certificates found")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load client keypair: %w", err)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ServerName:   serverName,
	}, nil
}

// Compile-time guard.
var _ StorageClient = (*GRPCStorageClient)(nil)
