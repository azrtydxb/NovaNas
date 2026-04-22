package metadata

import (
	"context"
	"fmt"
	"sync"

	pb "github.com/azrtydxb/novanas/storage/api/proto/metadata"
	"github.com/azrtydxb/novanas/storage/internal/disk"
)

// superblockProtosToScanResults converts a slice of wire-format
// SuperblockInfo messages into disk.ScanResult records suitable for the
// SuperblockRegistry. Invalid entries (missing uuid / bad digest length)
// are silently skipped.
func superblockProtosToScanResults(infos []*pb.SuperblockInfo) []disk.ScanResult {
	out := make([]disk.ScanResult, 0, len(infos))
	for _, info := range infos {
		if info == nil {
			continue
		}
		sb := &disk.Superblock{
			PoolID:              info.GetPoolId(),
			MetaVolumeName:      info.GetMetaVolumeName(),
			MetaVolumeRootChunk: info.GetMetaVolumeRoot(),
			MetaVolumeVersion:   info.GetMetaVolumeVersion(),
		}
		if uu := info.GetDiskUuid(); len(uu) == 16 {
			copy(sb.DiskUUID[:], uu)
		}
		if cd := info.GetCrushDigest(); len(cd) == 32 {
			copy(sb.CRUSHDigest[:], cd)
		}
		switch info.GetRole() {
		case "metadata":
			sb.Role = disk.DiskRoleMetadata
		case "both":
			sb.Role = disk.DiskRoleBoth
		case "data", "chunk":
			sb.Role = disk.DiskRoleData
		default:
			sb.Role = disk.DiskRoleUnknown
		}
		out = append(out, disk.ScanResult{
			Device:     disk.DeviceInfo{Path: info.GetDevicePath()},
			Superblock: sb,
			Status:     disk.DiskActive,
		})
	}
	return out
}

// SuperblockRegistry aggregates SuperblockInfo reports from agents and
// exposes them as a SuperblockSource for the metadata bootstrap. Reports
// are keyed by (nodeID, deviceUUID) so that repeated reports from the same
// disk overwrite the previous entry rather than duplicating.
//
// The registry is safe for concurrent use.
type SuperblockRegistry struct {
	mu      sync.Mutex
	entries map[string]disk.ScanResult // key = nodeID + "/" + hex(diskUUID)
	waiters []chan struct{}
}

// NewSuperblockRegistry constructs an empty registry.
func NewSuperblockRegistry() *SuperblockRegistry {
	return &SuperblockRegistry{entries: make(map[string]disk.ScanResult)}
}

// Ingest records a batch of per-disk reports from a single node. It
// returns the number of entries accepted and the new total count of
// metadata-role disks visible to the registry.
func (r *SuperblockRegistry) Ingest(nodeID string, reports []disk.ScanResult) (accepted int, metadataSeen int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rep := range reports {
		if rep.Superblock == nil {
			continue
		}
		key := fmt.Sprintf("%s/%x", nodeID, rep.Superblock.DiskUUID[:])
		r.entries[key] = rep
		accepted++
	}
	metadataSeen = r.metadataCountLocked()
	// Wake any pending waiters; they re-check predicates themselves.
	for _, ch := range r.waiters {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	return
}

// Snapshot returns a copy of all currently-known scan results, ordered
// non-deterministically.
func (r *SuperblockRegistry) Snapshot() []disk.ScanResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]disk.ScanResult, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	return out
}

func (r *SuperblockRegistry) metadataCountLocked() int {
	n := 0
	for _, e := range r.entries {
		if e.Superblock == nil {
			continue
		}
		if e.Superblock.Role == disk.DiskRoleMetadata || e.Superblock.Role == disk.DiskRoleBoth {
			n++
		}
	}
	return n
}

// GatherMetadataSuperblocks satisfies SuperblockSource. It blocks until
// at least minCount metadata-role reports have been ingested, or the
// context is cancelled.
func (r *SuperblockRegistry) GatherMetadataSuperblocks(ctx context.Context, minCount int) ([]disk.ScanResult, error) {
	for {
		r.mu.Lock()
		if r.metadataCountLocked() >= minCount {
			out := make([]disk.ScanResult, 0, len(r.entries))
			for _, e := range r.entries {
				out = append(out, e)
			}
			r.mu.Unlock()
			return out, nil
		}
		notify := make(chan struct{}, 1)
		r.waiters = append(r.waiters, notify)
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-notify:
			// re-check
		}
	}
}
