package metadata

import (
	"context"
	"testing"
	"time"

	pb "github.com/azrtydxb/novanas/storage/api/proto/metadata"
	"github.com/azrtydxb/novanas/storage/internal/disk"
)

func TestSuperblockRegistry_IngestAndGather(t *testing.T) {
	reg := NewSuperblockRegistry()

	infos := []*pb.SuperblockInfo{
		{
			DiskUuid:          make([]byte, 16),
			PoolId:            "p1",
			Role:              "metadata",
			CrushDigest:       make([]byte, 32),
			MetaVolumeName:    "meta",
			MetaVolumeRoot:    "root",
			MetaVolumeVersion: 1,
			DevicePath:        "/dev/nvme0n1",
		},
	}
	infos[0].DiskUuid[0] = 0x01
	accepted, seen := reg.Ingest("node-a", superblockProtosToScanResults(infos))
	if accepted != 1 {
		t.Fatalf("accepted=%d want 1", accepted)
	}
	if seen != 1 {
		t.Fatalf("metadata seen=%d want 1", seen)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	results, err := reg.GatherMetadataSuperblocks(ctx, 1)
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results)=%d", len(results))
	}
	if results[0].Superblock.Role != disk.DiskRoleMetadata {
		t.Fatalf("role mismatch: %v", results[0].Superblock.Role)
	}
}

func TestSuperblockRegistry_BlocksUntilQuorum(t *testing.T) {
	reg := NewSuperblockRegistry()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := reg.GatherMetadataSuperblocks(ctx, 2)
		done <- err
	}()

	time.Sleep(20 * time.Millisecond)
	// Ingest one disk: still below quorum.
	infos := []*pb.SuperblockInfo{{
		DiskUuid:    []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		Role:        "metadata",
		CrushDigest: make([]byte, 32),
	}}
	reg.Ingest("n1", superblockProtosToScanResults(infos))
	select {
	case <-done:
		t.Fatalf("gather returned before quorum")
	case <-time.After(50 * time.Millisecond):
	}
	// Ingest second disk: quorum met.
	infos2 := []*pb.SuperblockInfo{{
		DiskUuid:    []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		Role:        "both",
		CrushDigest: make([]byte, 32),
	}}
	reg.Ingest("n2", superblockProtosToScanResults(infos2))
	if err := <-done; err != nil {
		t.Fatalf("gather err: %v", err)
	}
}
