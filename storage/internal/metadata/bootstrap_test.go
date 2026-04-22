package metadata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/novanas/storage/internal/disk"
)

type fakeSource struct {
	results []disk.ScanResult
	err     error
}

func (f *fakeSource) GatherMetadataSuperblocks(ctx context.Context, min int) ([]disk.ScanResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func sb(name, root string, ver uint64, digest byte, role disk.DiskRole) disk.ScanResult {
	s := &disk.Superblock{
		Version:             disk.SuperblockVersion,
		PoolID:              "pool-default",
		Role:                role,
		MetaVolumeName:      name,
		MetaVolumeRootChunk: root,
		MetaVolumeVersion:   ver,
	}
	for i := range s.CRUSHDigest {
		s.CRUSHDigest[i] = digest
	}
	return disk.ScanResult{
		Device:     disk.DeviceInfo{Path: "/dev/fake"},
		Superblock: s,
		Status:     disk.DiskActive,
	}
}

func TestBootstrap_Success(t *testing.T) {
	src := &fakeSource{results: []disk.ScanResult{
		sb("meta", "rootA", 3, 0x42, disk.DiskRoleMetadata),
		sb("meta", "rootA", 3, 0x42, disk.DiskRoleBoth),
	}}
	cfg := BootstrapConfig{MinMetadataDisks: 2, BootstrapTimeout: time.Second}
	r, err := BootstrapChunkBacked(context.Background(), cfg, src)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if !r.CRUSHDigestAgreed {
		t.Error("expected CRUSH digest agreed")
	}
	if r.MetadataVolumeName != "meta" || r.MetadataVolumeRoot != "rootA" || r.MetadataVolumeVer != 3 {
		t.Errorf("unexpected report %+v", r)
	}
	if r.MetadataDisks != 2 {
		t.Errorf("disks = %d", r.MetadataDisks)
	}
}

func TestBootstrap_CRUSHDivergence(t *testing.T) {
	src := &fakeSource{results: []disk.ScanResult{
		sb("meta", "rootA", 1, 0x11, disk.DiskRoleMetadata),
		sb("meta", "rootA", 1, 0x22, disk.DiskRoleMetadata),
	}}
	cfg := BootstrapConfig{MinMetadataDisks: 2, BootstrapTimeout: time.Second}
	_, err := BootstrapChunkBacked(context.Background(), cfg, src)
	if !errors.Is(err, ErrBootstrapCRUSHDivergence) {
		t.Fatalf("want divergence error, got %v", err)
	}
}

func TestBootstrap_HighestVersionWins(t *testing.T) {
	src := &fakeSource{results: []disk.ScanResult{
		sb("meta", "old", 1, 0x77, disk.DiskRoleMetadata),
		sb("meta", "new", 5, 0x77, disk.DiskRoleBoth),
	}}
	cfg := BootstrapConfig{MinMetadataDisks: 2, BootstrapTimeout: time.Second}
	r, err := BootstrapChunkBacked(context.Background(), cfg, src)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if r.MetadataVolumeRoot != "new" || r.MetadataVolumeVer != 5 {
		t.Errorf("want newest locator, got %+v", r)
	}
}

func TestBootstrap_InsufficientDisks(t *testing.T) {
	src := &fakeSource{results: []disk.ScanResult{
		sb("meta", "rootA", 1, 0x99, disk.DiskRoleMetadata),
	}}
	cfg := BootstrapConfig{MinMetadataDisks: 3, BootstrapTimeout: 50 * time.Millisecond}
	_, err := BootstrapChunkBacked(context.Background(), cfg, src)
	if !errors.Is(err, ErrBootstrapTimeout) {
		t.Fatalf("want timeout, got %v", err)
	}
}

func TestBootstrap_MissingLocator(t *testing.T) {
	r := sb("", "", 0, 0x55, disk.DiskRoleMetadata)
	src := &fakeSource{results: []disk.ScanResult{r}}
	cfg := BootstrapConfig{MinMetadataDisks: 1, BootstrapTimeout: time.Second}
	_, err := BootstrapChunkBacked(context.Background(), cfg, src)
	if !errors.Is(err, ErrBootstrapMissingLocator) {
		t.Fatalf("want missing locator, got %v", err)
	}
}
