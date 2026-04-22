package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	pb "github.com/azrtydxb/novanas/storage/api/proto/metadata"
	"github.com/azrtydxb/novanas/storage/internal/metrics"
)

// VolumeProtectionMode is an alias for ProtectionMode for backward compatibility.
type VolumeProtectionMode = ProtectionMode

// DataProtectionConfig is an alias for ProtectionProfile for code clarity.
// In storage context, we talk about "data protection configuration"
// while in metadata context we use "protection profile".
type DataProtectionConfig = ProtectionProfile

// ReplicationConfig is an alias for ReplicationProfile.
type ReplicationConfig = ReplicationProfile

// ErasureCodingConfig is an alias for ErasureCodingProfile.
type ErasureCodingConfig = ErasureCodingProfile

// VolumeMeta stores metadata about a provisioned volume.
type VolumeMeta struct {
	VolumeID  string   `json:"volumeID"`
	Pool      string   `json:"pool"`
	SizeBytes uint64   `json:"sizeBytes"`
	ChunkIDs  []string `json:"chunkIDs"`

	// ChunkPlaintextHashes maps chunk id -> SHA-256(plaintext) for each
	// encrypted chunk in the volume. This is required on the read path
	// to re-derive the convergent chunk key (see
	// storage/internal/crypto/crypto.go, "Plaintext hash storage"). A
	// chunk id absent from this map indicates an unencrypted chunk and
	// is treated as a pass-through on read.
	//
	// Migration: chunks written before this field existed have no entry
	// and are therefore treated as unencrypted. This preserves backward
	// compatibility with pre-Wave-6 volumes.
	ChunkPlaintextHashes map[string][]byte `json:"chunkPlaintextHashes,omitempty"`

	// EncryptionEnabled indicates whether new chunks for this volume
	// should be encrypted. False (default) means unencrypted.
	EncryptionEnabled bool `json:"encryptionEnabled,omitempty"`

	// WrappedDK is the OpenBao Transit-wrapped Dataset Key for this
	// volume, stored in wrapped form. Empty when EncryptionEnabled is
	// false.
	WrappedDK []byte `json:"wrappedDK,omitempty"`

	// KeyVersion is the Transit master-key version that produced
	// WrappedDK. Callers must pass it back to VolumeKeyManager.Mount.
	KeyVersion uint64 `json:"keyVersion,omitempty"`

	// DataProtection specifies how the volume's data is protected.
	DataProtection *DataProtectionConfig `json:"dataProtection,omitempty"`

	// NVMe-oF target fields populated by the CSI controller after target creation.
	TargetNodeID  string `json:"targetNodeID,omitempty"`
	TargetAddress string `json:"targetAddress,omitempty"`
	TargetPort    string `json:"targetPort,omitempty"`
	SubsystemNQN  string `json:"subsystemNQN,omitempty"`

	// ProtectionProfile specifies the data protection settings for this volume.
	ProtectionProfile *ProtectionProfile `json:"protectionProfile,omitempty"`

	// ComplianceInfo tracks the current compliance state of this volume.
	ComplianceInfo *ComplianceInfo `json:"complianceInfo,omitempty"`
}

// SetChunkPlaintextHash records the plaintext hash for the given chunk
// id, allocating the map if needed. Safe to call on a nil receiver only
// for reads.
func (v *VolumeMeta) SetChunkPlaintextHash(chunkID string, plaintextHash []byte) {
	if v.ChunkPlaintextHashes == nil {
		v.ChunkPlaintextHashes = make(map[string][]byte)
	}
	b := make([]byte, len(plaintextHash))
	copy(b, plaintextHash)
	v.ChunkPlaintextHashes[chunkID] = b
}

// ChunkPlaintextHash returns the SHA-256(plaintext) recorded for
// chunkID, or (nil, false) if the chunk was stored unencrypted (or
// predates Wave 6).
func (v *VolumeMeta) ChunkPlaintextHash(chunkID string) ([]byte, bool) {
	if v == nil || v.ChunkPlaintextHashes == nil {
		return nil, false
	}
	h, ok := v.ChunkPlaintextHashes[chunkID]
	return h, ok
}

// PlacementMap records which nodes store replicas of a chunk.
type PlacementMap struct {
	ChunkID string   `json:"chunkID"`
	Nodes   []string `json:"nodes"`
}

// RaftStore is the NovaNas metadata store.
//
// The name is retained for historical continuity and to avoid churn at
// every call site; as of docs/14 decision S12, NovaNas is single-node by
// design and this store no longer uses Raft consensus. Operations are
// applied directly and synchronously to the underlying BadgerDB-backed
// FSM (or the in-memory FSM for tests), with durability provided by
// Badger's WAL + fsync.
//
// Future multi-node would reintroduce consensus (or move to
// metadata-as-chunks per S11), deferred.
type RaftStore struct {
	fsm MetadataFSM
}

// RaftConfig holds configuration for constructing a RaftStore.
//
// The Raft-specific fields (RaftAddr, RaftAdvertise, JoinAddrs,
// BootstrapExpect, GRPCDialOpts) are retained for API compatibility
// with existing callers but are ignored. They will be removed in a
// future cleanup pass.
type RaftConfig struct {
	// NodeID is a human-readable identifier for this metadata node.
	// Recorded in logs and metrics.
	NodeID string
	// DataDir is the directory where persistent state (BadgerDB files)
	// is stored.
	DataDir string
	// Backend selects the FSM storage backend. Valid values are "memory"
	// and "badger". When empty, defaults to "badger".
	Backend string

	// Deprecated: Raft has been removed (docs/14 S12). Retained for
	// source compatibility with existing callers.
	RaftAddr string
	// Deprecated: Raft has been removed (docs/14 S12).
	RaftAdvertise string
	// Deprecated: Raft has been removed (docs/14 S12).
	JoinAddrs string
	// Deprecated: Raft has been removed (docs/14 S12).
	BootstrapExpect int
	// Deprecated: Raft has been removed (docs/14 S12).
	GRPCDialOpts []grpc.DialOption
}

// NewRaftStore creates a metadata store. Despite the historical name,
// this constructor no longer configures Raft consensus — NovaNas is
// single-node (docs/14 S12) and metadata is persisted directly via the
// selected FSM backend.
func NewRaftStore(cfg RaftConfig) (*RaftStore, error) {
	var fsm MetadataFSM
	switch cfg.Backend {
	case "memory":
		fsm = NewFSM()
	case "badger", "":
		badgerDir := filepath.Join(cfg.DataDir, "badger")
		if err := os.MkdirAll(badgerDir, 0o750); err != nil {
			return nil, fmt.Errorf("creating badger data dir: %w", err)
		}
		badgerFSM, err := NewBadgerFSM(badgerDir)
		if err != nil {
			return nil, fmt.Errorf("creating badger fsm: %w", err)
		}
		fsm = badgerFSM
	default:
		return nil, fmt.Errorf("unknown backend %q: valid values are \"memory\" and \"badger\"", cfg.Backend)
	}

	return &RaftStore{fsm: fsm}, nil
}

// IsLeader always returns true on a single-node deployment. Retained for
// callers (e.g. the GC loop) that historically gated work on leadership.
func (s *RaftStore) IsLeader() bool {
	return true
}

// Close closes the underlying FSM, releasing resources.
func (s *RaftStore) Close() error {
	return s.fsm.Close()
}

// Backup writes a consistent snapshot of the metadata store to w. Since
// is the last version seen by a previous backup (0 for a full backup).
// Returns the last version included in the stream.
//
// Only the BadgerDB backend supports backup; the memory backend returns
// an error.
func (s *RaftStore) Backup(w io.Writer, since uint64) (uint64, error) {
	b, ok := s.fsm.(*BadgerFSM)
	if !ok {
		return 0, errors.New("backup is only supported on the badger backend")
	}
	return b.Backup(w, since)
}

// Restore replaces the metadata store contents with a previously
// produced Backup stream. Only the BadgerDB backend supports restore.
func (s *RaftStore) Restore(r io.Reader) error {
	b, ok := s.fsm.(*BadgerFSM)
	if !ok {
		return errors.New("restore is only supported on the badger backend")
	}
	return b.Restore(r)
}

// apply encodes op as a protobuf fsmOp and dispatches it directly to the
// FSM. Operations are synchronous and run on the caller's goroutine.
func (s *RaftStore) apply(op *fsmOp) error {
	metrics.MetadataOpsTotal.WithLabelValues("apply_" + op.Op).Inc()
	data, err := proto.Marshal(&pb.FsmOp{Op: op.Op, Bucket: op.Bucket, Key: op.Key, Value: op.Value})
	if err != nil {
		return fmt.Errorf("marshaling operation: %w", err)
	}
	resp := s.fsm.Apply(data)
	if resp != nil {
		if e, ok := resp.(error); ok {
			return e
		}
	}
	return nil
}

// applyWithResponse applies an operation and returns both the FSM response
// and any error. This is used for operations like AllocateIno that need to
// return a value from the FSM.
func (s *RaftStore) applyWithResponse(op *fsmOp) (interface{}, error) {
	metrics.MetadataOpsTotal.WithLabelValues("apply_" + op.Op).Inc()
	data, err := proto.Marshal(&pb.FsmOp{Op: op.Op, Bucket: op.Bucket, Key: op.Key, Value: op.Value})
	if err != nil {
		return nil, fmt.Errorf("marshaling operation: %w", err)
	}
	resp := s.fsm.Apply(data)
	if e, ok := resp.(error); ok {
		return nil, e
	}
	return resp, nil
}

// AllocateIno atomically allocates the next available inode number,
// ensuring uniqueness across restarts via the persistent counter bucket.
func (s *RaftStore) AllocateIno(_ context.Context) (uint64, error) {
	resp, err := s.applyWithResponse(&fsmOp{Op: opAllocateIno, Bucket: bucketCounters, Key: "nextIno"})
	if err != nil {
		return 0, fmt.Errorf("allocating inode: %w", err)
	}
	ino, ok := resp.(uint64)
	if !ok {
		return 0, fmt.Errorf("unexpected response type from AllocateIno FSM: %T", resp)
	}
	return ino, nil
}

// GetNextIno reads the current inode counter value from the FSM without
// incrementing it. Used at startup to seed the counter.
func (s *RaftStore) GetNextIno(_ context.Context) (uint64, error) {
	data, err := s.fsm.Get(bucketCounters, "nextIno")
	if err != nil {
		// Counter not yet initialized — return default.
		if errors.Is(err, ErrKeyNotFound) {
			return 2, nil
		}
		return 0, fmt.Errorf("reading inode counter: %w", err)
	}
	var val uint64
	if err := json.Unmarshal(data, &val); err != nil {
		return 0, fmt.Errorf("unmarshaling inode counter: %w", err)
	}
	return val, nil
}

// PutVolumeMeta stores volume metadata in the store.
func (s *RaftStore) PutVolumeMeta(_ context.Context, meta *VolumeMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling volume meta: %w", err)
	}
	return s.apply(&fsmOp{Op: opPut, Bucket: bucketVolumes, Key: meta.VolumeID, Value: data})
}

// GetVolumeMeta retrieves volume metadata by ID.
func (s *RaftStore) GetVolumeMeta(_ context.Context, volumeID string) (*VolumeMeta, error) {
	data, err := s.fsm.Get(bucketVolumes, volumeID)
	if err != nil {
		return nil, err
	}
	var meta VolumeMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshaling volume meta: %w", err)
	}
	return &meta, nil
}

// DeleteVolumeMeta removes volume metadata from the store.
func (s *RaftStore) DeleteVolumeMeta(_ context.Context, volumeID string) error {
	return s.apply(&fsmOp{Op: opDelete, Bucket: bucketVolumes, Key: volumeID})
}

// ListVolumesMeta returns all volume metadata entries.
func (s *RaftStore) ListVolumesMeta(_ context.Context) ([]*VolumeMeta, error) {
	all, err := s.fsm.GetAll(bucketVolumes)
	if err != nil {
		return nil, fmt.Errorf("listing volumes: %w", err)
	}
	result := make([]*VolumeMeta, 0, len(all))
	for _, data := range all {
		var meta VolumeMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("unmarshaling volume meta: %w", err)
		}
		result = append(result, &meta)
	}
	return result, nil
}

// PutPlacementMap stores a chunk's placement map.
func (s *RaftStore) PutPlacementMap(_ context.Context, pm *PlacementMap) error {
	data, err := json.Marshal(pm)
	if err != nil {
		return fmt.Errorf("marshaling placement map: %w", err)
	}
	return s.apply(&fsmOp{Op: opPut, Bucket: bucketPlacements, Key: pm.ChunkID, Value: data})
}

// GetPlacementMap retrieves a chunk's placement map.
func (s *RaftStore) GetPlacementMap(_ context.Context, chunkID string) (*PlacementMap, error) {
	data, err := s.fsm.Get(bucketPlacements, chunkID)
	if err != nil {
		return nil, err
	}
	var pm PlacementMap
	if err := json.Unmarshal(data, &pm); err != nil {
		return nil, fmt.Errorf("unmarshaling placement map: %w", err)
	}
	return &pm, nil
}

// ListPlacementMaps returns all placement map entries.
func (s *RaftStore) ListPlacementMaps(_ context.Context) ([]*PlacementMap, error) {
	all, err := s.fsm.GetAll(bucketPlacements)
	if err != nil {
		return nil, fmt.Errorf("listing placement maps: %w", err)
	}
	result := make([]*PlacementMap, 0, len(all))
	for _, data := range all {
		var pm PlacementMap
		if err := json.Unmarshal(data, &pm); err != nil {
			return nil, fmt.Errorf("unmarshaling placement map: %w", err)
		}
		result = append(result, &pm)
	}
	return result, nil
}

// DeletePlacementMap removes a placement map entry.
func (s *RaftStore) DeletePlacementMap(_ context.Context, chunkID string) error {
	return s.apply(&fsmOp{Op: opDelete, Bucket: bucketPlacements, Key: chunkID})
}

// ---- Lock lease operations ----

// AcquireLock attempts to acquire a file lock lease. If successful, returns the lease ID.
// If a conflicting lock exists, returns an error with the conflicting owner.
func (s *RaftStore) AcquireLock(ctx context.Context, args *AcquireLockArgs) (*AcquireLockResult, error) {
	// First check for conflicts by reading existing locks for this inode.
	locks, err := s.getLocksForInode(ctx, args.Ino)
	if err != nil {
		return nil, fmt.Errorf("checking existing locks: %w", err)
	}

	// Filter out expired locks and check for conflicts.
	candidateLock := &LockLease{
		Owner:     args.Owner,
		VolumeID:  args.VolumeID,
		Ino:       args.Ino,
		Start:     args.Start,
		End:       args.End,
		Type:      args.Type,
		ExpiresAt: time.Now().Add(args.TTL).UnixNano(),
		FilerID:   args.FilerID,
	}

	for _, existing := range locks {
		if existing.IsExpired() {
			// Clean up expired lock asynchronously.
			go s.ReleaseLock(context.Background(), &ReleaseLockArgs{
				LeaseID: existing.LeaseID,
				Owner:   existing.Owner,
			})
			continue
		}
		if conflicts(candidateLock, existing) {
			return &AcquireLockResult{
					ConflictingOwner: existing.Owner,
				}, fmt.Errorf("lock conflict: owner %q holds conflicting %s lock on inode %d [%d,%d)",
					existing.Owner, existing.Type, existing.Ino, existing.Start, existing.End)
		}
	}

	// No conflicts, create the lease.
	leaseID := GenerateLeaseID()
	candidateLock.LeaseID = leaseID

	// Store the lease.
	leaseData, err := json.Marshal(candidateLock)
	if err != nil {
		return nil, fmt.Errorf("marshaling lease: %w", err)
	}
	if err := s.apply(&fsmOp{Op: opPut, Bucket: bucketLocks, Key: lockKey(leaseID), Value: leaseData}); err != nil {
		return nil, fmt.Errorf("storing lease: %w", err)
	}

	// Update the index.
	if err := s.addLockToIndex(ctx, args.Ino, leaseID); err != nil {
		// Best effort cleanup on index update failure.
		_ = s.apply(&fsmOp{Op: opDelete, Bucket: bucketLocks, Key: lockKey(leaseID)})
		return nil, fmt.Errorf("updating lock index: %w", err)
	}

	return &AcquireLockResult{
		LeaseID:   leaseID,
		ExpiresAt: candidateLock.ExpiresAt,
	}, nil
}

// RenewLock extends the expiration time of an existing lock lease.
func (s *RaftStore) RenewLock(_ context.Context, args *RenewLockArgs) (*LockLease, error) {
	leaseData, err := s.fsm.Get(bucketLocks, lockKey(args.LeaseID))
	if err != nil {
		return nil, fmt.Errorf("lease not found: %w", err)
	}

	var lease LockLease
	if err := json.Unmarshal(leaseData, &lease); err != nil {
		return nil, fmt.Errorf("unmarshaling lease: %w", err)
	}

	if lease.IsExpired() {
		return nil, fmt.Errorf("cannot renew expired lease %s", args.LeaseID)
	}

	// Update expiration.
	lease.ExpiresAt = time.Now().Add(args.TTL).UnixNano()

	newLeaseData, err := json.Marshal(lease)
	if err != nil {
		return nil, fmt.Errorf("marshaling lease: %w", err)
	}
	if err := s.apply(&fsmOp{Op: opPut, Bucket: bucketLocks, Key: lockKey(args.LeaseID), Value: newLeaseData}); err != nil {
		return nil, fmt.Errorf("storing renewed lease: %w", err)
	}

	return &lease, nil
}

// ReleaseLock releases a lock lease, removing it from the metadata store.
func (s *RaftStore) ReleaseLock(ctx context.Context, args *ReleaseLockArgs) error {
	// Verify the lease exists and belongs to the owner (if specified).
	leaseData, err := s.fsm.Get(bucketLocks, lockKey(args.LeaseID))
	if err != nil {
		return fmt.Errorf("lease not found: %w", err)
	}

	var lease LockLease
	if err := json.Unmarshal(leaseData, &lease); err != nil {
		return fmt.Errorf("unmarshaling lease: %w", err)
	}

	if args.Owner != "" && lease.Owner != args.Owner {
		return fmt.Errorf("lease owner mismatch: expected %q, got %q", lease.Owner, args.Owner)
	}

	// Remove the lease.
	if err := s.apply(&fsmOp{Op: opDelete, Bucket: bucketLocks, Key: lockKey(args.LeaseID)}); err != nil {
		return fmt.Errorf("deleting lease: %w", err)
	}

	// Remove from index.
	if err := s.removeLockFromIndex(ctx, lease.Ino, args.LeaseID); err != nil {
		// Log but don't fail - stale index entries are cleaned up during reads.
		return fmt.Errorf("removing from index: %w", err)
	}

	return nil
}

// TestLock checks if a lock could be acquired without actually acquiring it.
// Returns nil if no conflict exists, or the conflicting lock if one does.
func (s *RaftStore) TestLock(ctx context.Context, args *TestLockArgs) (*LockLease, error) {
	locks, err := s.getLocksForInode(ctx, args.Ino)
	if err != nil {
		return nil, fmt.Errorf("checking existing locks: %w", err)
	}

	candidateLock := &LockLease{
		VolumeID: args.VolumeID,
		Ino:      args.Ino,
		Start:    args.Start,
		End:      args.End,
		Type:     args.Type,
	}

	for _, existing := range locks {
		if existing.IsExpired() {
			continue
		}
		if conflicts(candidateLock, existing) {
			return existing, nil
		}
	}

	return nil, nil
}

// GetLock retrieves a lock lease by ID.
func (s *RaftStore) GetLock(_ context.Context, leaseID string) (*LockLease, error) {
	leaseData, err := s.fsm.Get(bucketLocks, lockKey(leaseID))
	if err != nil {
		return nil, fmt.Errorf("lease not found: %w", err)
	}

	var lease LockLease
	if err := json.Unmarshal(leaseData, &lease); err != nil {
		return nil, fmt.Errorf("unmarshaling lease: %w", err)
	}

	return &lease, nil
}

// ListLocks returns all active (non-expired) locks, optionally filtered by volume ID.
func (s *RaftStore) ListLocks(_ context.Context, volumeID string) ([]*LockLease, error) {
	all, err := s.fsm.GetAll(bucketLocks)
	if err != nil {
		return nil, fmt.Errorf("listing locks: %w", err)
	}

	var result []*LockLease
	now := time.Now().UnixNano()

	for _, data := range all {
		var lease LockLease
		if err := json.Unmarshal(data, &lease); err != nil {
			continue // Skip corrupted entries.
		}
		if lease.ExpiresAt < now {
			continue // Skip expired leases.
		}
		if volumeID != "" && lease.VolumeID != volumeID {
			continue // Filter by volume.
		}
		result = append(result, &lease)
	}

	return result, nil
}

// CleanupExpiredLocks removes expired lock leases. Should be called periodically.
func (s *RaftStore) CleanupExpiredLocks(ctx context.Context) (int, error) {
	all, err := s.fsm.GetAll(bucketLocks)
	if err != nil {
		return 0, fmt.Errorf("listing locks: %w", err)
	}

	now := time.Now().UnixNano()
	cleaned := 0

	for key, data := range all {
		var lease LockLease
		if err := json.Unmarshal(data, &lease); err != nil {
			continue
		}
		if lease.ExpiresAt < now {
			// Remove expired lease.
			if err := s.apply(&fsmOp{Op: opDelete, Bucket: bucketLocks, Key: key}); err == nil {
				_ = s.removeLockFromIndex(ctx, lease.Ino, lease.LeaseID)
				cleaned++
			}
		}
	}

	return cleaned, nil
}

// getLocksForInode retrieves all locks for a given inode.
// Returns nil, nil if no locks exist for this inode (not found is OK).
func (s *RaftStore) getLocksForInode(_ context.Context, ino uint64) ([]*LockLease, error) {
	// Get the index for this inode.
	indexData, err := s.fsm.Get(bucketLocks, lockIndexKey(ino))
	if err != nil {
		// No locks for this inode yet - not found is acceptable.
		if errors.Is(err, ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting lock index: %w", err)
	}

	var index lockIndex
	if err := json.Unmarshal(indexData, &index); err != nil {
		return nil, fmt.Errorf("unmarshaling lock index: %w", err)
	}

	var locks []*LockLease
	for _, leaseID := range index.LeaseIDs {
		leaseData, err := s.fsm.Get(bucketLocks, lockKey(leaseID))
		if err != nil {
			// Lease may have been deleted, skip.
			continue
		}
		var lease LockLease
		if err := json.Unmarshal(leaseData, &lease); err != nil {
			continue
		}
		locks = append(locks, &lease)
	}

	return locks, nil
}

// addLockToIndex adds a lease ID to the inode's lock index.
func (s *RaftStore) addLockToIndex(_ context.Context, ino uint64, leaseID string) error {
	indexData, err := s.fsm.Get(bucketLocks, lockIndexKey(ino))
	if err != nil {
		// No index yet, create a new one.
		index := lockIndex{
			Ino:      ino,
			LeaseIDs: []string{leaseID},
		}
		data, err := json.Marshal(index)
		if err != nil {
			return fmt.Errorf("marshaling new index: %w", err)
		}
		return s.apply(&fsmOp{Op: opPut, Bucket: bucketLocks, Key: lockIndexKey(ino), Value: data})
	}

	var index lockIndex
	if err := json.Unmarshal(indexData, &index); err != nil {
		return fmt.Errorf("unmarshaling index: %w", err)
	}

	// Check for duplicates.
	for _, existingID := range index.LeaseIDs {
		if existingID == leaseID {
			return nil // Already in index.
		}
	}

	index.LeaseIDs = append(index.LeaseIDs, leaseID)
	data, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("marshaling updated index: %w", err)
	}
	return s.apply(&fsmOp{Op: opPut, Bucket: bucketLocks, Key: lockIndexKey(ino), Value: data})
}

// removeLockFromIndex removes a lease ID from the inode's lock index.
func (s *RaftStore) removeLockFromIndex(_ context.Context, ino uint64, leaseID string) error {
	indexData, err := s.fsm.Get(bucketLocks, lockIndexKey(ino))
	if err != nil {
		// Index doesn't exist, nothing to do - not found is acceptable.
		if errors.Is(err, ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("getting lock index: %w", err)
	}

	var index lockIndex
	if err := json.Unmarshal(indexData, &index); err != nil {
		return fmt.Errorf("unmarshaling index: %w", err)
	}

	// Filter out the lease ID.
	newLeaseIDs := make([]string, 0, len(index.LeaseIDs))
	found := false
	for _, existingID := range index.LeaseIDs {
		if existingID != leaseID {
			newLeaseIDs = append(newLeaseIDs, existingID)
		} else {
			found = true
		}
	}

	if !found {
		return nil // Lease ID not in index.
	}

	if len(newLeaseIDs) == 0 {
		// Remove the index entry entirely.
		return s.apply(&fsmOp{Op: opDelete, Bucket: bucketLocks, Key: lockIndexKey(ino)})
	}

	index.LeaseIDs = newLeaseIDs
	data, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("marshaling updated index: %w", err)
	}
	return s.apply(&fsmOp{Op: opPut, Bucket: bucketLocks, Key: lockIndexKey(ino), Value: data})
}

// splitAndTrim splits a comma-separated list of addresses and trims whitespace.
// Retained for backward compatibility with tests; no longer used internally
// now that cluster joining is a no-op on a single-node deployment.
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// StartMetricsMonitor is retained for API compatibility. Previously it
// reported Raft-specific state; on a single-node deployment there is
// nothing dynamic to report, so this is now a no-op that respects the
// caller-provided context for cancellation.
func (s *RaftStore) StartMetricsMonitor(_ context.Context, _ time.Duration) {
	// No-op: NovaNas is single-node (docs/14 S12); the former Raft state
	// metrics (leader/follower, commit index) are no longer meaningful.
}
