package metadata

import (
	"bytes"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"

	pb "github.com/azrtydxb/novanas/storage/api/proto/metadata"
)

func newTestBadgerFSM(t *testing.T) (*BadgerFSM, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "novanas-badger-test-*")
	if err != nil {
		t.Fatal(err)
	}
	fsm, err := NewBadgerFSM(dir)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("NewBadgerFSM: %v", err)
	}
	return fsm, func() {
		fsm.Close()
		os.RemoveAll(dir)
	}
}

func applyOp(t *testing.T, f MetadataFSM, op *fsmOp) {
	t.Helper()
	data, err := proto.Marshal(&pb.FsmOp{Op: op.Op, Bucket: op.Bucket, Key: op.Key, Value: op.Value})
	if err != nil {
		t.Fatalf("marshal fsmOp: %v", err)
	}
	resp := f.Apply(data)
	if resp != nil {
		if e, ok := resp.(error); ok {
			t.Fatalf("Apply returned error: %v", e)
		}
	}
}

func TestBadgerFSM_PutAndGet(t *testing.T) {
	f, cleanup := newTestBadgerFSM(t)
	defer cleanup()

	applyOp(t, f, &fsmOp{Op: opPut, Bucket: "test-bucket", Key: "key1", Value: []byte("value1")})

	got, err := f.Get("test-bucket", "key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "value1" {
		t.Errorf("expected %q, got %q", "value1", string(got))
	}
}

func TestBadgerFSM_GetNotFound(t *testing.T) {
	f, cleanup := newTestBadgerFSM(t)
	defer cleanup()

	_, err := f.Get("no-bucket", "no-key")
	if err == nil {
		t.Error("expected error for missing key")
	}
}

func TestBadgerFSM_Delete(t *testing.T) {
	f, cleanup := newTestBadgerFSM(t)
	defer cleanup()

	applyOp(t, f, &fsmOp{Op: opPut, Bucket: "b", Key: "k", Value: []byte("v")})

	// Verify it exists.
	if _, err := f.Get("b", "k"); err != nil {
		t.Fatalf("expected key to exist: %v", err)
	}

	// Delete.
	applyOp(t, f, &fsmOp{Op: opDelete, Bucket: "b", Key: "k"})

	// Verify it is gone.
	_, err := f.Get("b", "k")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestBadgerFSM_GetAll(t *testing.T) {
	f, cleanup := newTestBadgerFSM(t)
	defer cleanup()

	// GetAll on an empty bucket returns nil.
	got, err := f.GetAll("empty")
	if err != nil {
		t.Fatalf("GetAll empty: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for empty bucket, got %v", got)
	}

	// Insert entries.
	applyOp(t, f, &fsmOp{Op: opPut, Bucket: "items", Key: "a", Value: []byte("va")})
	applyOp(t, f, &fsmOp{Op: opPut, Bucket: "items", Key: "b", Value: []byte("vb")})
	applyOp(t, f, &fsmOp{Op: opPut, Bucket: "items", Key: "c", Value: []byte("vc")})

	all, err := f.GetAll("items")
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(all))
	}
	if string(all["a"]) != "va" || string(all["b"]) != "vb" || string(all["c"]) != "vc" {
		t.Errorf("unexpected entries: %v", all)
	}
}

func TestBadgerFSM_MultipleBuckets(t *testing.T) {
	f, cleanup := newTestBadgerFSM(t)
	defer cleanup()

	// Put entries into two different buckets with the same key.
	applyOp(t, f, &fsmOp{Op: opPut, Bucket: "alpha", Key: "key1", Value: []byte("alpha-val")})
	applyOp(t, f, &fsmOp{Op: opPut, Bucket: "beta", Key: "key1", Value: []byte("beta-val")})

	// Each bucket should only see its own entry.
	alphaAll, err := f.GetAll("alpha")
	if err != nil {
		t.Fatalf("GetAll alpha: %v", err)
	}
	if len(alphaAll) != 1 {
		t.Fatalf("expected 1 entry in alpha, got %d", len(alphaAll))
	}
	if string(alphaAll["key1"]) != "alpha-val" {
		t.Errorf("alpha value mismatch: %q", string(alphaAll["key1"]))
	}

	betaAll, err := f.GetAll("beta")
	if err != nil {
		t.Fatalf("GetAll beta: %v", err)
	}
	if len(betaAll) != 1 {
		t.Fatalf("expected 1 entry in beta, got %d", len(betaAll))
	}
	if string(betaAll["key1"]) != "beta-val" {
		t.Errorf("beta value mismatch: %q", string(betaAll["key1"]))
	}

	// Verify individual Get returns correct values per bucket.
	gotAlpha, err := f.Get("alpha", "key1")
	if err != nil {
		t.Fatalf("Get alpha/key1: %v", err)
	}
	if string(gotAlpha) != "alpha-val" {
		t.Errorf("expected alpha-val, got %q", string(gotAlpha))
	}

	gotBeta, err := f.Get("beta", "key1")
	if err != nil {
		t.Fatalf("Get beta/key1: %v", err)
	}
	if string(gotBeta) != "beta-val" {
		t.Errorf("expected beta-val, got %q", string(gotBeta))
	}

	// Delete from one bucket should not affect the other.
	applyOp(t, f, &fsmOp{Op: opDelete, Bucket: "alpha", Key: "key1"})

	_, err = f.Get("alpha", "key1")
	if err == nil {
		t.Error("expected error after deleting from alpha")
	}
	gotBetaAfter, err := f.Get("beta", "key1")
	if err != nil {
		t.Fatalf("beta/key1 should still exist: %v", err)
	}
	if string(gotBetaAfter) != "beta-val" {
		t.Errorf("beta value changed after alpha delete: %q", string(gotBetaAfter))
	}
}

// TestBadgerFSM_BackupAndRestore exercises the native Badger backup/restore
// flow that replaced the former Raft snapshot mechanism (docs/14 S12).
func TestBadgerFSM_BackupAndRestore(t *testing.T) {
	f1, cleanup1 := newTestBadgerFSM(t)
	defer cleanup1()

	// Populate some data across multiple buckets.
	entries := []fsmOp{
		{Op: opPut, Bucket: "volumes", Key: "vol-1", Value: []byte(`{"id":"vol-1"}`)},
		{Op: opPut, Bucket: "volumes", Key: "vol-2", Value: []byte(`{"id":"vol-2"}`)},
		{Op: opPut, Bucket: "placements", Key: "chunk-1", Value: []byte(`{"nodes":["a","b"]}`)},
		{Op: opPut, Bucket: "objects", Key: "bucket/key", Value: []byte(`{"size":100}`)},
	}
	for i := range entries {
		applyOp(t, f1, &entries[i])
	}

	// Take a full backup.
	var buf bytes.Buffer
	if _, err := f1.Backup(&buf, 0); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	// Create a new BadgerFSM and restore the backup into it.
	f2, cleanup2 := newTestBadgerFSM(t)
	defer cleanup2()

	if err := f2.Restore(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Verify all original data is present.
	for _, e := range entries {
		got, err := f2.Get(e.Bucket, e.Key)
		if err != nil {
			t.Errorf("Get(%s, %s) after restore: %v", e.Bucket, e.Key, err)
			continue
		}
		if !bytes.Equal(got, e.Value) {
			t.Errorf("Get(%s, %s) = %q, want %q", e.Bucket, e.Key, got, e.Value)
		}
	}
}

func TestBadgerFSM_UnknownOp(t *testing.T) {
	f, cleanup := newTestBadgerFSM(t)
	defer cleanup()

	data, _ := proto.Marshal(&pb.FsmOp{Op: "invalid", Bucket: "b", Key: "k"})
	resp := f.Apply(data)
	if resp == nil {
		t.Fatal("expected error for unknown op")
	}
	if _, ok := resp.(error); !ok {
		t.Fatalf("expected error, got %T", resp)
	}
}

func TestBadgerFSM_InvalidLogData(t *testing.T) {
	f, cleanup := newTestBadgerFSM(t)
	defer cleanup()

	resp := f.Apply([]byte("not protobuf"))
	if resp == nil {
		t.Fatal("expected error for invalid protobuf")
	}
	if _, ok := resp.(error); !ok {
		t.Fatalf("expected error, got %T", resp)
	}
}
