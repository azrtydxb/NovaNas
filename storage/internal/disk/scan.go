package disk

import (
	"errors"
	"fmt"
)

// ScanResult records the outcome of reading a superblock from one device.
// A disk is ACTIVE when its superblock is present and valid; IDENTIFIED
// when the device was enumerated but has no valid superblock (admin must
// initialise it).
type ScanResult struct {
	Device     DeviceInfo
	Superblock *Superblock // nil when Status != DiskActive
	Status     DiskStatus
	Err        error
}

// DiskStatus is the bootstrap-time status of a local disk.
type DiskStatus int

const (
	// DiskUnknownStatus is the zero value.
	DiskUnknownStatus DiskStatus = iota
	// DiskActive: superblock valid; disk participates in its pool.
	DiskActive
	// DiskIdentified: device present but superblock missing / corrupt;
	// admin must assign (WriteSuperblock) before use.
	DiskIdentified
	// DiskError: I/O error reading the device.
	DiskError
)

func (s DiskStatus) String() string {
	switch s {
	case DiskActive:
		return "active"
	case DiskIdentified:
		return "identified"
	case DiskError:
		return "error"
	default:
		return "unknown"
	}
}

// ScanSuperblocks reads the superblock from each device in infos and
// classifies the result. It never panics; per-device errors are captured
// in ScanResult.Err.
func ScanSuperblocks(infos []DeviceInfo) []ScanResult {
	out := make([]ScanResult, 0, len(infos))
	for _, d := range infos {
		out = append(out, scanOne(d))
	}
	return out
}

func scanOne(d DeviceInfo) ScanResult {
	sb, err := ReadSuperblock(d.Path)
	if err == nil {
		return ScanResult{Device: d, Superblock: sb, Status: DiskActive}
	}
	// Missing magic or bad CRC => identified (not assigned).
	if errors.Is(err, ErrSuperblockBadMagic) || errors.Is(err, ErrSuperblockBadCRC) || errors.Is(err, ErrSuperblockBadVersion) {
		return ScanResult{Device: d, Status: DiskIdentified, Err: err}
	}
	// Any other error (I/O, permissions) — mark as error.
	return ScanResult{Device: d, Status: DiskError, Err: fmt.Errorf("superblock read: %w", err)}
}

// MetadataPoolCandidates returns only those scan results that declare a
// metadata role. Used by the meta bootstrap path to gather superblocks
// pointing at the metadata BlockVolume.
func MetadataPoolCandidates(results []ScanResult) []ScanResult {
	var out []ScanResult
	for _, r := range results {
		if r.Status != DiskActive || r.Superblock == nil {
			continue
		}
		if r.Superblock.Role == DiskRoleMetadata || r.Superblock.Role == DiskRoleBoth {
			out = append(out, r)
		}
	}
	return out
}
