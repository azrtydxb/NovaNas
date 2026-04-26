// Package metadata — bootstrap.
//
// Architecture note (A4-Metadata-As-Chunks, docs/02, docs/14 S14):
//
// The metadata service itself lives on the chunk engine (its BadgerDB
// files are stored on a BlockVolume whose backing chunks are replicated
// across metadata-role disks). This creates a chicken-and-egg problem at
// startup: we need metadata to know where the metadata volume's chunks
// are, but metadata is not yet available.
//
// The solution is the per-disk superblock (see storage/internal/disk).
// At startup the metadata service:
//
//  1. Waits for agents to report their local superblocks (heartbeat).
//  2. Gathers superblocks from disks with role ∈ {metadata, both}.
//  3. Verifies they agree on a single CRUSH-map digest (divergence
//     indicates a split-brain situation — abort and alert).
//  4. Reads the "metadata volume locator" (name + root chunk ID + version)
//     from the superblocks — any one is authoritative once CRUSH digests
//     agree.
//  5. Assembles the BlockVolume, exposes it as a local block device
//     (loopback or NBD), formats with xfs on first use, and mounts at
//     --meta-mount-path.
//  6. Opens BadgerDB at the mount path.
//  7. Serves gRPC as normal.
//
// This file contains the Go-side control flow. The actual BlockVolume
// assembly / mount step is delegated to the Rust data-plane via gRPC
// (TODO(integration): wiring to be completed once the NBD bdev path in
// storage/dataplane/nbd/ supports BlockVolume export).
package metadata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/azrtydxb/novanas/storage/internal/disk"
)

// BootstrapConfig controls the metadata-service bootstrap sequence.
type BootstrapConfig struct {
	// LocalDataDir is the fallback BadgerDB path used when
	// ChunkBackedEnabled is false (legacy mode). Kept for backward
	// compatibility with the pre-A4-Metadata-As-Chunks layout.
	LocalDataDir string

	// ChunkBackedEnabled selects the new chunk-backed mode. When false,
	// meta behaves like the A4-Single-Node implementation (BadgerDB on
	// local disk).
	ChunkBackedEnabled bool

	// MetaMountPath is where the chunk-backed metadata BlockVolume is
	// mounted; BadgerDB is opened inside this directory.
	MetaMountPath string

	// BootstrapTimeout is how long to wait for enough superblocks to be
	// reported before giving up and failing startup.
	BootstrapTimeout time.Duration

	// MinMetadataDisks is the number of metadata-role disks that must be
	// visible before bootstrap proceeds (quorum-equivalent).
	MinMetadataDisks int
}

// BootstrapReport summarises what the bootstrap sequence observed.
type BootstrapReport struct {
	MetadataDisks       int
	MetadataVolumeName  string
	MetadataVolumeRoot  string
	MetadataVolumeVer   uint64
	CRUSHDigestAgreed   bool
	AgreedCRUSHDigestHx string
}

// Errors.
var (
	ErrBootstrapTimeout          = errors.New("metadata bootstrap: timed out waiting for superblocks")
	ErrBootstrapCRUSHDivergence  = errors.New("metadata bootstrap: superblocks disagree on CRUSH map digest")
	ErrBootstrapMissingLocator   = errors.New("metadata bootstrap: no metadata-role disks carry a volume locator")
	ErrBootstrapNotYetIntegrated = errors.New("metadata bootstrap: chunk-backed mount path not yet integrated")
)

// SuperblockSource is the interface the bootstrap sequence uses to read
// superblocks reported by agents. A test or a real gRPC-backed
// implementation can satisfy it.
type SuperblockSource interface {
	// GatherMetadataSuperblocks returns scan results from metadata-role
	// disks currently reported by agents. Implementations may block while
	// accumulating reports; callers time the overall call out using
	// BootstrapConfig.BootstrapTimeout.
	GatherMetadataSuperblocks(ctx context.Context, minCount int) ([]disk.ScanResult, error)
}

// BootstrapChunkBacked runs the chunk-backed bootstrap sequence and
// returns a BootstrapReport. The return value describes what was observed
// but the final step (mount + BadgerDB open on the mounted volume) is
// intentionally left as a TODO(integration) for callers who must wire in
// the NBD/loopback export path from the data-plane.
//
// Callers that cannot yet satisfy the integration contract should use
// BootstrapLocal for now; the deprecation log lives at the call site.
func BootstrapChunkBacked(ctx context.Context, cfg BootstrapConfig, src SuperblockSource) (*BootstrapReport, error) {
	if cfg.MinMetadataDisks <= 0 {
		cfg.MinMetadataDisks = 1
	}
	if cfg.BootstrapTimeout <= 0 {
		cfg.BootstrapTimeout = 30 * time.Second
	}

	gatherCtx, cancel := context.WithTimeout(ctx, cfg.BootstrapTimeout)
	defer cancel()

	results, err := src.GatherMetadataSuperblocks(gatherCtx, cfg.MinMetadataDisks)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrBootstrapTimeout
		}
		return nil, fmt.Errorf("gathering superblocks: %w", err)
	}
	candidates := disk.MetadataPoolCandidates(results)
	if len(candidates) < cfg.MinMetadataDisks {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrBootstrapTimeout, len(candidates), cfg.MinMetadataDisks)
	}

	report := &BootstrapReport{MetadataDisks: len(candidates)}

	// Verify CRUSH-digest agreement.
	first := candidates[0].Superblock
	agreed := first.CRUSHDigest
	for _, r := range candidates[1:] {
		if r.Superblock.CRUSHDigest != agreed {
			return nil, fmt.Errorf("%w: disk %s diverges", ErrBootstrapCRUSHDivergence, r.Device.Path)
		}
	}
	report.CRUSHDigestAgreed = true
	report.AgreedCRUSHDigestHx = fmt.Sprintf("%x", agreed[:])

	// Find the highest MetaVolumeVersion across candidates; that wins in
	// case of per-disk staleness (admin just updated the locator).
	winner := first
	for _, r := range candidates[1:] {
		if r.Superblock.MetaVolumeVersion > winner.MetaVolumeVersion {
			winner = r.Superblock
		}
	}
	if winner.MetaVolumeName == "" || winner.MetaVolumeRootChunk == "" {
		return nil, ErrBootstrapMissingLocator
	}
	report.MetadataVolumeName = winner.MetaVolumeName
	report.MetadataVolumeRoot = winner.MetaVolumeRootChunk
	report.MetadataVolumeVer = winner.MetaVolumeVersion

	// The NBD-backed BlockVolume export is implemented in
	// storage/dataplane/src/transport/dataplane_service.rs::
	// export_metadata_volume_nbd (gated behind the spdk-sys feature)
	// and consumed via DataplaneNBDMounter (see cmd/meta/main.go).
	// The mount + mkfs + Badger-open stages happen in
	// bootstrap_mount.go::MountAndOpen — see
	// NewRaftStoreChunkBackedMounted for the fully-wired entry point.
	// This function is the pure superblock-resolution step; callers
	// chain it with the mounter via bootstrapWithMounter (#13).

	return report, nil
}

// NewRaftStoreChunkBacked is a thin wrapper that runs BootstrapChunkBacked
// and then opens the metadata store. When the chunk-mount step is still
// stubbed (see TODO above), it falls back to opening BadgerDB at
// cfg.LocalDataDir and returns both the store and the bootstrap report so
// callers can log the observation.
func NewRaftStoreChunkBacked(ctx context.Context, nodeID string, cfg BootstrapConfig, src SuperblockSource) (*RaftStore, *BootstrapReport, error) {
	report, err := BootstrapChunkBacked(ctx, cfg, src)
	if err != nil {
		return nil, nil, err
	}
	// The mount step is wired in bootstrap_mount.go. This legacy
	// helper is preserved for tests that exercise the bootstrap
	// resolution path without the SPDK mounter; production callers
	// use NewRaftStoreChunkBackedMounted, which opens BadgerDB on
	// the mounted volume (see bootstrap_mount.go) and falls back to
	// LocalDataDir only when the mounter is unsupported.
	store, err := NewRaftStore(RaftConfig{
		NodeID:  nodeID,
		DataDir: cfg.LocalDataDir,
	})
	if err != nil {
		return nil, report, fmt.Errorf("opening metadata store: %w", err)
	}
	return store, report, nil
}
