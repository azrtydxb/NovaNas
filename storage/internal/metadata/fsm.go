// Package metadata: single-node design (docs/14 S12).
//
// NovaNas is single-node by design. Metadata writes go directly to the
// underlying store (BadgerDB) with durability provided by Badger's WAL +
// fsync. Raft consensus was removed because it added operational
// complexity without value on a single-node deployment.
//
// The FSM type retains its name for historical continuity; it is no
// longer a raft.FSM and no longer participates in log replication. It
// is a plain key-value state machine that decodes an encoded fsmOp
// payload and applies it synchronously.
//
// Future multi-node would reintroduce consensus or move to
// metadata-as-chunks (S11), deferred.
package metadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	pb "github.com/azrtydxb/novanas/storage/api/proto/metadata"
)

const (
	opPut         = "put"
	opDelete      = "delete"
	opAddQuota    = "addQuota"
	opSubQuota    = "subQuota"
	opAllocateIno = "allocateIno"

	bucketVolumes          = "volumes"
	bucketCounters         = "counters"
	bucketPlacements       = "placements"
	bucketObjects          = "objects"
	bucketBuckets          = "buckets" // S3 buckets, not FSM buckets
	bucketMultipart        = "multipart"
	bucketSnapshots        = "snapshots"
	bucketShardPlacements  = "shardPlacements"
	bucketVolumeCompliance = "volumeCompliance"
	bucketHealTasks        = "healTasks"
	bucketChunkHealLocks   = "chunkHealLocks"
	bucketLocks            = "locks" // File lock leases
	bucketQuotas           = "quotas"
	bucketUsage            = "usage"
)

// Sentinel errors for FSM operations.
var (
	// ErrBucketNotFound is returned when attempting to access a non-existent bucket.
	ErrBucketNotFound = errors.New("bucket not found")
	// ErrKeyNotFound is returned when a key does not exist in a bucket.
	ErrKeyNotFound = errors.New("key not found")
)

// MetadataFSM defines the interface that both the in-memory FSM and the
// BadgerDB-backed FSM implement. It provides synchronous Apply and
// typed read accessors plus a Close method for resource cleanup.
type MetadataFSM interface {
	// Apply decodes and applies an encoded fsmOp payload. Returns a
	// response value (e.g. allocated inode) or an error.
	Apply(data []byte) interface{}
	Get(bucket, key string) ([]byte, error)
	GetAll(bucket string) (map[string][]byte, error)
	Close() error
}

type fsmOp struct {
	Op     string `json:"op"`
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
	Value  []byte `json:"value,omitempty"`
}

// decodeFsmOp decodes a protobuf-encoded fsmOp, falling back to JSON
// for backward compatibility with legacy payloads.
func decodeFsmOp(data []byte) (fsmOp, error) {
	var op fsmOp
	var pbOp pb.FsmOp
	if err := proto.Unmarshal(data, &pbOp); err != nil {
		if jsonErr := json.Unmarshal(data, &op); jsonErr != nil {
			return op, fmt.Errorf("unmarshaling fsm op: %w", err)
		}
		return op, nil
	}
	return fsmOp{Op: pbOp.Op, Bucket: pbOp.Bucket, Key: pbOp.Key, Value: pbOp.Value}, nil
}

// FSM is the in-memory implementation of MetadataFSM. It stores all metadata
// in nested maps protected by a read-write mutex. Suitable for testing and
// small deployments where persistence is not required.
type FSM struct {
	mu      sync.RWMutex
	buckets map[string]map[string][]byte
}

// Compile-time check that FSM implements MetadataFSM.
var _ MetadataFSM = (*FSM)(nil)

// NewFSM creates a new in-memory FSM with all required buckets initialized.
func NewFSM() *FSM {
	return &FSM{
		buckets: map[string]map[string][]byte{
			bucketVolumes:          {},
			bucketPlacements:       {},
			bucketObjects:          {},
			bucketBuckets:          {},
			bucketMultipart:        {},
			bucketSnapshots:        {},
			bucketShardPlacements:  {},
			bucketVolumeCompliance: {},
			bucketHealTasks:        {},
			bucketChunkHealLocks:   {},
			bucketLocks:            {},
			bucketCounters:         {},
			bucketQuotas:           {},
			bucketUsage:            {},
			"nodes":                {},
			"inodes":               {},
			"dirents":              {},
		},
	}
}

// Apply decodes and applies an encoded fsmOp payload against the in-memory state.
func (f *FSM) Apply(data []byte) interface{} {
	op, err := decodeFsmOp(data)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	bucket, ok := f.buckets[op.Bucket]
	if !ok {
		f.buckets[op.Bucket] = make(map[string][]byte)
		bucket = f.buckets[op.Bucket]
	}
	switch op.Op {
	case opPut:
		bucket[op.Key] = op.Value
	case opDelete:
		delete(bucket, op.Key)
	case opAddQuota:
		var delta int64
		if err := json.Unmarshal(op.Value, &delta); err != nil {
			return fmt.Errorf("unmarshaling quota delta: %w", err)
		}
		current := int64(0)
		if existing, ok := bucket[op.Key]; ok {
			if err := json.Unmarshal(existing, &current); err != nil {
				return fmt.Errorf("unmarshaling current usage: %w", err)
			}
		}
		newVal := current + delta
		if newVal < 0 {
			newVal = 0
		}
		updated, _ := json.Marshal(newVal)
		bucket[op.Key] = updated
	case opSubQuota:
		var delta int64
		if err := json.Unmarshal(op.Value, &delta); err != nil {
			return fmt.Errorf("unmarshaling quota delta: %w", err)
		}
		current := int64(0)
		if existing, ok := bucket[op.Key]; ok {
			if err := json.Unmarshal(existing, &current); err != nil {
				return fmt.Errorf("unmarshaling current usage: %w", err)
			}
		}
		newVal := current - delta
		if newVal < 0 {
			newVal = 0
		}
		updated, _ := json.Marshal(newVal)
		bucket[op.Key] = updated
	case opAllocateIno:
		current := uint64(2) // Default start value (1 is reserved for root).
		if existing, ok := bucket[op.Key]; ok {
			if err := json.Unmarshal(existing, &current); err != nil {
				return fmt.Errorf("unmarshaling current inode counter: %w", err)
			}
		}
		next := current + 1
		updated, _ := json.Marshal(next)
		bucket[op.Key] = updated
		return next
	default:
		return fmt.Errorf("unknown op: %s", op.Op)
	}
	return nil
}

// Get retrieves a value by key from the specified bucket.
func (f *FSM) Get(bucket, key string) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	b, ok := f.buckets[bucket]
	if !ok {
		return nil, fmt.Errorf("%w: bucket %s", ErrBucketNotFound, bucket)
	}
	data, ok := b[key]
	if !ok {
		return nil, fmt.Errorf("%w: key %s not found in bucket %s", ErrKeyNotFound, key, bucket)
	}
	return data, nil
}

// GetAll retrieves all key-value pairs from the specified bucket.
func (f *FSM) GetAll(bucket string) (map[string][]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	b, ok := f.buckets[bucket]
	if !ok {
		return nil, nil
	}
	cp := make(map[string][]byte, len(b))
	for k, v := range b {
		val := make([]byte, len(v))
		copy(val, v)
		cp[k] = val
	}
	return cp, nil
}

// Close is a no-op for the in-memory FSM since there are no resources to release.
func (f *FSM) Close() error {
	return nil
}
