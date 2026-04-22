// Package disk — superblock.
//
// Architecture note (A4-Metadata-As-Chunks, docs/02 "Bootstrap", docs/14 S14):
//
// The superblock is the ONLY non-chunk data on any NovaNas disk. It solves
// the chicken-and-egg bootstrap problem: the metadata service lives on
// chunks, but to find those chunks the cluster must first enumerate its
// disks and learn which pool they belong to. Each disk therefore carries a
// small (4 KiB), fixed-offset, CRC-protected descriptor at the start of the
// device.
//
// Layout (little-endian, fixed offsets):
//
//	offset  size  field
//	------  ----  -------------------------------------------------
//	0       8     magic   ("NOVANAS\x00")
//	8       4     version (currently 1)
//	12      4     flags   (reserved, 0)
//	16      16    disk UUID (binary, matches WWN-derived identity)
//	32      32    pool ID  (utf-8, zero-padded)
//	64      4     role     (1 = data, 2 = metadata, 3 = both)
//	68      32    CRUSH map digest (sha256 of canonical CRUSH map)
//	100     32    metadata volume name (utf-8, zero-padded)
//	132     64    metadata volume root chunk ID (hex sha256, zero-padded)
//	196     8     metadata volume version (monotonic counter)
//	204     8     created_unix_nanos
//	212     8     updated_unix_nanos
//	220     3872  reserved (zeroed)
//	4092    4     crc32c over bytes [0..4092)
//
// The superblock is written at byte offset 0 of the raw device. For GPT
// co-existence, higher deployments may opt to shift the superblock to a
// reserved area; the WriteAt/ReadAt offset is centralised in
// SuperblockOffset for that reason.
package disk

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"time"
)

// SuperblockSize is the on-disk size of the superblock (4 KiB).
const SuperblockSize = 4096

// SuperblockOffset is the byte offset on the raw device where the
// superblock lives. Zero by default; shift if you need to leave room for
// a partition table.
const SuperblockOffset int64 = 0

// SuperblockMagic is the magic number ("NOVANAS\x00"). 8 bytes.
var SuperblockMagic = [8]byte{'N', 'O', 'V', 'A', 'N', 'A', 'S', 0}

// SuperblockVersion is the current on-disk format version.
const SuperblockVersion uint32 = 1

// DiskRole is the role a disk plays in its pool.
type DiskRole uint32

const (
	// DiskRoleUnknown is the zero value; invalid at runtime.
	DiskRoleUnknown DiskRole = 0
	// DiskRoleData participates only in data pools.
	DiskRoleData DiskRole = 1
	// DiskRoleMetadata participates only in the metadata pool.
	DiskRoleMetadata DiskRole = 2
	// DiskRoleBoth participates in both data and metadata pools.
	DiskRoleBoth DiskRole = 3
)

// Superblock is the in-memory form of the on-disk superblock.
type Superblock struct {
	Version  uint32
	Flags    uint32
	DiskUUID [16]byte
	PoolID   string // max 32 bytes when serialised
	Role     DiskRole

	// CRUSHDigest is a SHA-256 (32 bytes) of the canonical CRUSH map. Used
	// to detect divergence across disks when a meta-quorum is assembled.
	CRUSHDigest [32]byte

	// Metadata-volume locator: how to reconstruct the metadata BlockVolume.
	MetaVolumeName      string // max 32 bytes
	MetaVolumeRootChunk string // max 64 bytes (hex sha256)
	MetaVolumeVersion   uint64

	CreatedUnixNanos int64
	UpdatedUnixNanos int64
}

// Field offsets — keep in sync with the layout comment above.
const (
	sbOffMagic          = 0
	sbOffVersion        = 8
	sbOffFlags          = 12
	sbOffDiskUUID       = 16
	sbOffPoolID         = 32
	sbOffRole           = 64
	sbOffCRUSHDigest    = 68
	sbOffMetaVolName    = 100
	sbOffMetaVolRoot    = 132
	sbOffMetaVolVersion = 196
	sbOffCreated        = 204
	sbOffUpdated        = 212
	sbOffCRC            = 4092

	sbFieldPoolIDLen      = 32
	sbFieldMetaVolNameLen = 32
	sbFieldMetaVolRootLen = 64
)

// Errors.
var (
	ErrSuperblockBadMagic   = errors.New("superblock magic mismatch")
	ErrSuperblockBadCRC     = errors.New("superblock CRC mismatch")
	ErrSuperblockBadVersion = errors.New("superblock version unsupported")
	ErrFieldTooLong         = errors.New("field exceeds its on-disk capacity")
)

// Marshal returns the fixed-size 4096-byte on-disk representation.
func (s *Superblock) Marshal() ([]byte, error) {
	if len(s.PoolID) > sbFieldPoolIDLen {
		return nil, fmt.Errorf("%w: pool id (%d > %d)", ErrFieldTooLong, len(s.PoolID), sbFieldPoolIDLen)
	}
	if len(s.MetaVolumeName) > sbFieldMetaVolNameLen {
		return nil, fmt.Errorf("%w: meta volume name", ErrFieldTooLong)
	}
	if len(s.MetaVolumeRootChunk) > sbFieldMetaVolRootLen {
		return nil, fmt.Errorf("%w: meta volume root chunk id", ErrFieldTooLong)
	}

	buf := make([]byte, SuperblockSize)
	copy(buf[sbOffMagic:sbOffMagic+8], SuperblockMagic[:])
	binary.LittleEndian.PutUint32(buf[sbOffVersion:], s.Version)
	binary.LittleEndian.PutUint32(buf[sbOffFlags:], s.Flags)
	copy(buf[sbOffDiskUUID:sbOffDiskUUID+16], s.DiskUUID[:])
	copy(buf[sbOffPoolID:sbOffPoolID+sbFieldPoolIDLen], []byte(s.PoolID))
	binary.LittleEndian.PutUint32(buf[sbOffRole:], uint32(s.Role))
	copy(buf[sbOffCRUSHDigest:sbOffCRUSHDigest+32], s.CRUSHDigest[:])
	copy(buf[sbOffMetaVolName:sbOffMetaVolName+sbFieldMetaVolNameLen], []byte(s.MetaVolumeName))
	copy(buf[sbOffMetaVolRoot:sbOffMetaVolRoot+sbFieldMetaVolRootLen], []byte(s.MetaVolumeRootChunk))
	binary.LittleEndian.PutUint64(buf[sbOffMetaVolVersion:], s.MetaVolumeVersion)
	binary.LittleEndian.PutUint64(buf[sbOffCreated:], uint64(s.CreatedUnixNanos))
	binary.LittleEndian.PutUint64(buf[sbOffUpdated:], uint64(s.UpdatedUnixNanos))

	table := crc32.MakeTable(crc32.Castagnoli)
	crc := crc32.Checksum(buf[:sbOffCRC], table)
	binary.LittleEndian.PutUint32(buf[sbOffCRC:], crc)
	return buf, nil
}

// UnmarshalSuperblock parses a 4096-byte buffer into a Superblock.
// Returns ErrSuperblockBadMagic / ErrSuperblockBadCRC / ErrSuperblockBadVersion
// on validation failure.
func UnmarshalSuperblock(buf []byte) (*Superblock, error) {
	if len(buf) != SuperblockSize {
		return nil, fmt.Errorf("superblock buffer size %d, want %d", len(buf), SuperblockSize)
	}
	var magic [8]byte
	copy(magic[:], buf[sbOffMagic:sbOffMagic+8])
	if magic != SuperblockMagic {
		return nil, ErrSuperblockBadMagic
	}

	table := crc32.MakeTable(crc32.Castagnoli)
	gotCRC := binary.LittleEndian.Uint32(buf[sbOffCRC:])
	wantCRC := crc32.Checksum(buf[:sbOffCRC], table)
	if gotCRC != wantCRC {
		return nil, fmt.Errorf("%w: got=%08x want=%08x", ErrSuperblockBadCRC, gotCRC, wantCRC)
	}

	s := &Superblock{
		Version: binary.LittleEndian.Uint32(buf[sbOffVersion:]),
		Flags:   binary.LittleEndian.Uint32(buf[sbOffFlags:]),
	}
	if s.Version != SuperblockVersion {
		return nil, fmt.Errorf("%w: got=%d want=%d", ErrSuperblockBadVersion, s.Version, SuperblockVersion)
	}
	copy(s.DiskUUID[:], buf[sbOffDiskUUID:sbOffDiskUUID+16])
	s.PoolID = trimZero(buf[sbOffPoolID : sbOffPoolID+sbFieldPoolIDLen])
	s.Role = DiskRole(binary.LittleEndian.Uint32(buf[sbOffRole:]))
	copy(s.CRUSHDigest[:], buf[sbOffCRUSHDigest:sbOffCRUSHDigest+32])
	s.MetaVolumeName = trimZero(buf[sbOffMetaVolName : sbOffMetaVolName+sbFieldMetaVolNameLen])
	s.MetaVolumeRootChunk = trimZero(buf[sbOffMetaVolRoot : sbOffMetaVolRoot+sbFieldMetaVolRootLen])
	s.MetaVolumeVersion = binary.LittleEndian.Uint64(buf[sbOffMetaVolVersion:])
	s.CreatedUnixNanos = int64(binary.LittleEndian.Uint64(buf[sbOffCreated:]))
	s.UpdatedUnixNanos = int64(binary.LittleEndian.Uint64(buf[sbOffUpdated:]))
	return s, nil
}

// trimZero strips trailing NUL bytes from a fixed-width utf-8 field.
func trimZero(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// WriteSuperblock writes the superblock to the device at SuperblockOffset.
// Fills in Version / UpdatedUnixNanos automatically. Opens the device with
// O_WRONLY; caller is responsible for not writing to a mounted filesystem
// (use a dedicated raw disk).
func WriteSuperblock(devicePath string, s *Superblock) error {
	if s.Version == 0 {
		s.Version = SuperblockVersion
	}
	now := time.Now().UnixNano()
	if s.CreatedUnixNanos == 0 {
		s.CreatedUnixNanos = now
	}
	s.UpdatedUnixNanos = now

	buf, err := s.Marshal()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(devicePath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("opening %s for superblock write: %w", devicePath, err)
	}
	defer f.Close()
	if _, err := f.WriteAt(buf, SuperblockOffset); err != nil {
		return fmt.Errorf("writing superblock to %s: %w", devicePath, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing superblock on %s: %w", devicePath, err)
	}
	return nil
}

// ReadSuperblock reads and validates the superblock at SuperblockOffset.
func ReadSuperblock(devicePath string) (*Superblock, error) {
	f, err := os.Open(devicePath)
	if err != nil {
		return nil, fmt.Errorf("opening %s for superblock read: %w", devicePath, err)
	}
	defer f.Close()
	buf := make([]byte, SuperblockSize)
	if _, err := f.ReadAt(buf, SuperblockOffset); err != nil {
		return nil, fmt.Errorf("reading superblock from %s: %w", devicePath, err)
	}
	return UnmarshalSuperblock(buf)
}
