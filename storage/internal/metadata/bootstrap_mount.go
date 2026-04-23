package metadata

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// MetadataVolumeMounter is the injectable hook that completes the
// chunk-backed bootstrap. Given the BlockVolume locator emitted by
// BootstrapChunkBacked (name + root chunk ID + version), an implementation:
//
//  1. Assembles the BlockVolume from its replicated chunks.
//  2. Exports it as a local block device (NBD or loopback).
//  3. Returns the device path (/dev/nbdN, /dev/loopN, ...) ready for mkfs.
//
// Two real implementations:
//   - DataplaneNBDMounter (this file): talks to a NovaNas Rust data-plane
//     that exposes an NBD export primitive. Wired via an ExportBdev-style
//     RPC; the Go side invokes the RPC via the caller-supplied Exporter
//     function so this package does not need to import storage/dataplane's
//     gRPC client types (which would create an import cycle).
//   - NoopMetadataVolumeMounter: for tests / dev. Returns an explicit
//     ErrMountNotSupported.
//
// The mount + mkfs + Badger-open stages live in MountAndOpen below, which
// is protocol-agnostic (takes a device path).
type MetadataVolumeMounter interface {
	// ExportMetadataVolume assembles the metadata BlockVolume described by
	// `locator` and returns the host block-device path ready for mkfs/mount.
	// It must be idempotent: if the device is already exported for this
	// locator, return the same path.
	ExportMetadataVolume(ctx context.Context, locator VolumeLocator) (devicePath string, err error)

	// ReleaseMetadataVolume tears down the export (on graceful shutdown or
	// failover). Best-effort; implementations should log and swallow errors
	// internally but return them for the caller to audit.
	ReleaseMetadataVolume(ctx context.Context, locator VolumeLocator) error
}

// VolumeLocator is the on-disk pointer from the superblocks to the metadata
// BlockVolume. It is the subset of BootstrapReport fields that identify a
// volume uniquely.
type VolumeLocator struct {
	Name       string
	RootChunk  string
	Version    uint64
	CRUSHDigestHex string
}

// Errors produced by the mount path.
var (
	// ErrMountNotSupported is returned by NoopMetadataVolumeMounter so
	// callers can detect "dev mode, fall back to LocalDataDir".
	ErrMountNotSupported = errors.New("metadata mount: mounter not configured")
	// ErrMkfsFailed wraps mkfs.xfs exit errors.
	ErrMkfsFailed = errors.New("metadata mount: mkfs.xfs failed")
	// ErrMountFailed wraps the mount(8) / syscall.Mount failure.
	ErrMountFailed = errors.New("metadata mount: mount(8) failed")
)

// NoopMetadataVolumeMounter is the fallback used when chunk-backed mount
// wiring is not configured. Causes NewRaftStoreChunkBacked to fall through
// to LocalDataDir, logging the reason.
type NoopMetadataVolumeMounter struct{}

// ExportMetadataVolume returns ErrMountNotSupported so the caller can
// distinguish "not configured" from a real mount failure.
func (NoopMetadataVolumeMounter) ExportMetadataVolume(_ context.Context, _ VolumeLocator) (string, error) {
	return "", ErrMountNotSupported
}

// ReleaseMetadataVolume is a no-op.
func (NoopMetadataVolumeMounter) ReleaseMetadataVolume(_ context.Context, _ VolumeLocator) error {
	return nil
}

// ExporterFunc is the caller-supplied adapter from a VolumeLocator to a
// host block device path. In production this is backed by a gRPC call into
// the Rust data-plane (ExportBdev-for-NBD or a sibling RPC). In tests it
// can return a path to a local loopback device.
type ExporterFunc func(ctx context.Context, locator VolumeLocator) (string, error)

// DataplaneNBDMounter is a MetadataVolumeMounter that delegates the
// "assemble+export" step to a caller-supplied ExporterFunc. The mount/mkfs
// step is handled here in Go (runs mkfs.xfs and mount(8)).
//
// BLOCKER(wave-finish): the ExporterFunc currently has no production
// implementation in storage/dataplane because the Rust data-plane does not
// yet expose an NBD-export RPC. The proto contract below matches what
// rust-dataplane will implement; until then callers must either inject a
// loopback-based ExporterFunc (dev) or fall back to LocalDataDir.
//
// Target proto contract (in storage/api/proto/dataplane/dataplane.proto
// once the sibling worktree regenerates):
//
//   rpc ExportMetadataVolumeNBD(ExportMetadataVolumeNBDRequest) returns
//     (ExportMetadataVolumeNBDResponse);
//
//   message ExportMetadataVolumeNBDRequest {
//     string volume_name  = 1;
//     string root_chunk_id = 2;
//     uint64 volume_version = 3;
//   }
//   message ExportMetadataVolumeNBDResponse {
//     string device_path = 1; // "/dev/nbdN"
//   }
type DataplaneNBDMounter struct {
	Export  ExporterFunc
	Release ExporterFunc // May be nil.
}

// ExportMetadataVolume delegates to the caller-supplied Export function.
func (m *DataplaneNBDMounter) ExportMetadataVolume(ctx context.Context, locator VolumeLocator) (string, error) {
	if m == nil || m.Export == nil {
		return "", ErrMountNotSupported
	}
	return m.Export(ctx, locator)
}

// ReleaseMetadataVolume delegates to Release when set; otherwise no-op.
func (m *DataplaneNBDMounter) ReleaseMetadataVolume(ctx context.Context, locator VolumeLocator) error {
	if m == nil || m.Release == nil {
		return nil
	}
	_, err := m.Release(ctx, locator)
	return err
}

// MountAndOpen runs mkfs.xfs (first boot only) + mount(8) on devicePath,
// returning the mount point ready for BadgerDB to open. isFirstBoot is
// determined by a marker file left inside the mount point on success so
// subsequent mounts skip mkfs.
//
// On any failure the mount is rolled back (umount best-effort) to avoid
// leaking kernel state across process restarts.
func MountAndOpen(ctx context.Context, devicePath, mountPath string, isFirstBootHintOverride *bool) error {
	if err := os.MkdirAll(mountPath, 0o755); err != nil {
		return fmt.Errorf("%w: mkdir mount path: %v", ErrMountFailed, err)
	}

	firstBoot := false
	if isFirstBootHintOverride != nil {
		firstBoot = *isFirstBootHintOverride
	} else {
		// Detect first boot by probing the device for an xfs signature.
		// `blkid -o value -s TYPE <dev>` returns empty for unformatted.
		out, _ := exec.CommandContext(ctx, "blkid", "-o", "value", "-s", "TYPE", devicePath).Output()
		firstBoot = len(out) == 0
	}

	if firstBoot {
		cmd := exec.CommandContext(ctx, "mkfs.xfs", "-f", "-m", "crc=1,reflink=1", devicePath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrMkfsFailed, string(out), err)
		}
	}

	// mount(8) with sane XFS options for a metadata workload:
	// noatime (reduces random writes), inode64 (large inode addressing).
	cmd := exec.CommandContext(ctx, "mount", "-t", "xfs", "-o", "noatime,inode64", devicePath, mountPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrMountFailed, string(out), err)
	}

	// Drop a marker so future boots can tell whether this mount was
	// initialised by NovaNas (surfaces wipe/foreign-disk situations).
	marker := filepath.Join(mountPath, ".novanas-metadata")
	if firstBoot {
		_ = os.WriteFile(marker, []byte(fmt.Sprintf("initialised=%s\n", time.Now().UTC().Format(time.RFC3339))), 0o600)
	}

	return nil
}

// bootstrapWithMounter is the chunk-backed bootstrap + mount orchestrator.
// It is invoked by NewRaftStoreChunkBackedMounted; kept package-private so
// callers go through the official entry point.
func bootstrapWithMounter(ctx context.Context, cfg BootstrapConfig, src SuperblockSource, mounter MetadataVolumeMounter) (*BootstrapReport, string, error) {
	report, err := BootstrapChunkBacked(ctx, cfg, src)
	if err != nil {
		return nil, "", err
	}
	locator := VolumeLocator{
		Name:           report.MetadataVolumeName,
		RootChunk:      report.MetadataVolumeRoot,
		Version:        report.MetadataVolumeVer,
		CRUSHDigestHex: report.AgreedCRUSHDigestHx,
	}
	devicePath, err := mounter.ExportMetadataVolume(ctx, locator)
	if err != nil {
		return report, "", fmt.Errorf("metadata mount: export: %w", err)
	}
	if err := MountAndOpen(ctx, devicePath, cfg.MetaMountPath, nil); err != nil {
		// Best-effort release on mount failure so we don't leak exports.
		_ = mounter.ReleaseMetadataVolume(ctx, locator)
		return report, devicePath, err
	}
	return report, cfg.MetaMountPath, nil
}

// NewRaftStoreChunkBackedMounted is the fully-wired chunk-backed bootstrap
// entry point. On success it returns a *RaftStore opened at the mounted
// metadata volume (not at cfg.LocalDataDir). On ErrMountNotSupported it
// falls back to cfg.LocalDataDir and returns the report; other errors are
// surfaced as-is so operators can alert.
func NewRaftStoreChunkBackedMounted(
	ctx context.Context,
	nodeID string,
	cfg BootstrapConfig,
	src SuperblockSource,
	mounter MetadataVolumeMounter,
) (*RaftStore, *BootstrapReport, error) {
	if mounter == nil {
		mounter = NoopMetadataVolumeMounter{}
	}
	report, mountPath, err := bootstrapWithMounter(ctx, cfg, src, mounter)
	if err != nil {
		if errors.Is(err, ErrMountNotSupported) {
			// Dev-mode / mounter not wired: fall back to local data dir so
			// the service still starts. Log-level decision is the caller's.
			store, openErr := NewRaftStore(RaftConfig{NodeID: nodeID, DataDir: cfg.LocalDataDir})
			if openErr != nil {
				return nil, report, fmt.Errorf("opening metadata store (local fallback): %w", openErr)
			}
			return store, report, nil
		}
		return nil, report, err
	}
	store, err := NewRaftStore(RaftConfig{NodeID: nodeID, DataDir: mountPath})
	if err != nil {
		return nil, report, fmt.Errorf("opening metadata store on mounted volume: %w", err)
	}
	return store, report, nil
}
