package metadata

import (
	"encoding/json"
	"fmt"
	"io"

	badger "github.com/dgraph-io/badger/v4"
)

// BadgerFSM is a persistent implementation of MetadataFSM backed by BadgerDB.
// Keys are stored as composite "bucket:key" strings to partition data into
// logical buckets while keeping a single key-value namespace.
//
// Durability is provided by BadgerDB's native WAL + fsync — NovaNas is
// single-node by design (docs/14 S12) and does not need Raft consensus.
type BadgerFSM struct {
	db *badger.DB
}

// Compile-time check that BadgerFSM implements MetadataFSM.
var _ MetadataFSM = (*BadgerFSM)(nil)

// NewBadgerFSM opens (or creates) a BadgerDB database at the given directory
// path and returns a BadgerFSM ready for use.
func NewBadgerFSM(dir string) (*BadgerFSM, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil) // Suppress BadgerDB's internal logging.
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("opening badger db at %s: %w", dir, err)
	}
	return &BadgerFSM{db: db}, nil
}

// compositeKey builds the "bucket:key" composite key used for BadgerDB storage.
func compositeKey(bucket, key string) []byte {
	return []byte(bucket + ":" + key)
}

// bucketPrefix returns the prefix used to scan all keys within a bucket.
func bucketPrefix(bucket string) []byte {
	return []byte(bucket + ":")
}

// Apply decodes an encoded fsmOp payload and executes it against BadgerDB.
// Returns nil on success, a non-error response (e.g. allocated inode), or
// an error.
func (f *BadgerFSM) Apply(data []byte) interface{} {
	op, err := decodeFsmOp(data)
	if err != nil {
		return err
	}

	switch op.Op {
	case opPut:
		err := f.db.Update(func(txn *badger.Txn) error {
			return txn.Set(compositeKey(op.Bucket, op.Key), op.Value)
		})
		if err != nil {
			return fmt.Errorf("badger put: %w", err)
		}
	case opDelete:
		err := f.db.Update(func(txn *badger.Txn) error {
			return txn.Delete(compositeKey(op.Bucket, op.Key))
		})
		if err != nil {
			return fmt.Errorf("badger delete: %w", err)
		}
	case opAddQuota:
		err := f.db.Update(func(txn *badger.Txn) error {
			var delta int64
			if err := json.Unmarshal(op.Value, &delta); err != nil {
				return fmt.Errorf("unmarshaling quota delta: %w", err)
			}
			current := int64(0)
			item, err := txn.Get(compositeKey(op.Bucket, op.Key))
			if err == nil {
				val, err := item.ValueCopy(nil)
				if err == nil {
					if err := json.Unmarshal(val, &current); err != nil {
						return fmt.Errorf("unmarshaling current usage: %w", err)
					}
				}
			}
			newVal := current + delta
			if newVal < 0 {
				newVal = 0
			}
			updated, _ := json.Marshal(newVal)
			return txn.Set(compositeKey(op.Bucket, op.Key), updated)
		})
		if err != nil {
			return fmt.Errorf("badger add quota: %w", err)
		}
	case opSubQuota:
		err := f.db.Update(func(txn *badger.Txn) error {
			var delta int64
			if err := json.Unmarshal(op.Value, &delta); err != nil {
				return fmt.Errorf("unmarshaling quota delta: %w", err)
			}
			current := int64(0)
			item, err := txn.Get(compositeKey(op.Bucket, op.Key))
			if err == nil {
				val, err := item.ValueCopy(nil)
				if err == nil {
					if err := json.Unmarshal(val, &current); err != nil {
						return fmt.Errorf("unmarshaling current usage: %w", err)
					}
				}
			}
			newVal := current - delta
			if newVal < 0 {
				newVal = 0
			}
			updated, _ := json.Marshal(newVal)
			return txn.Set(compositeKey(op.Bucket, op.Key), updated)
		})
		if err != nil {
			return fmt.Errorf("badger sub quota: %w", err)
		}
	case opAllocateIno:
		var next uint64
		err := f.db.Update(func(txn *badger.Txn) error {
			current := uint64(2) // Default start value.
			item, err := txn.Get(compositeKey(op.Bucket, op.Key))
			if err == nil {
				val, err := item.ValueCopy(nil)
				if err == nil {
					if err := json.Unmarshal(val, &current); err != nil {
						return fmt.Errorf("unmarshaling current inode counter: %w", err)
					}
				}
			}
			next = current + 1
			updated, _ := json.Marshal(next)
			return txn.Set(compositeKey(op.Bucket, op.Key), updated)
		})
		if err != nil {
			return fmt.Errorf("badger allocate ino: %w", err)
		}
		return next
	default:
		return fmt.Errorf("unknown op: %s", op.Op)
	}
	return nil
}

// Get retrieves a single value from the specified bucket and key.
func (f *BadgerFSM) Get(bucket, key string) ([]byte, error) {
	var val []byte
	err := f.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(compositeKey(bucket, key))
		if err != nil {
			return err
		}
		val, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, fmt.Errorf("%w: key %s not found in bucket %s", ErrKeyNotFound, key, bucket)
		}
		return nil, fmt.Errorf("badger get: %w", err)
	}
	return val, nil
}

// GetAll returns all key-value pairs within the specified bucket. The returned
// map uses the original key (without the bucket prefix). If the bucket has no
// entries, nil is returned.
func (f *BadgerFSM) GetAll(bucket string) (map[string][]byte, error) {
	prefix := bucketPrefix(bucket)
	prefixLen := len(prefix)
	var result map[string][]byte

	err := f.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			originalKey := string(k[prefixLen:])
			val, err := item.ValueCopy(nil)
			if err != nil {
				return fmt.Errorf("reading value for key %s: %w", originalKey, err)
			}
			if result == nil {
				result = make(map[string][]byte)
			}
			result[originalKey] = val
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("badger prefix scan: %w", err)
	}
	return result, nil
}

// Backup writes a consistent snapshot of the database to w. Since is the
// last version seen by a previous backup (0 for a full backup). Returns the
// last version included in the stream, suitable for incremental follow-ups.
//
// This replaces the former Raft snapshot mechanism: operators use Backup for
// disaster-recovery dumps (see Wave 7 ConfigBackupPolicy) and Restore to
// reload from such a dump.
func (f *BadgerFSM) Backup(w io.Writer, since uint64) (uint64, error) {
	return f.db.Backup(w, since)
}

// Restore loads a backup stream produced by Backup, replacing the current
// database contents. Existing keys are overwritten by entries in the stream.
func (f *BadgerFSM) Restore(r io.Reader) error {
	return f.db.Load(r, 16)
}

// Close closes the underlying BadgerDB database, releasing all resources.
func (f *BadgerFSM) Close() error {
	return f.db.Close()
}
