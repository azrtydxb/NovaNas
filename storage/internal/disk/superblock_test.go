package disk

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func makeSB() *Superblock {
	s := &Superblock{
		Version:             SuperblockVersion,
		PoolID:              "pool-default",
		Role:                DiskRoleBoth,
		MetaVolumeName:      "meta-vol",
		MetaVolumeRootChunk: "abc123def456",
		MetaVolumeVersion:   42,
	}
	for i := range s.DiskUUID {
		s.DiskUUID[i] = byte(i + 1)
	}
	for i := range s.CRUSHDigest {
		s.CRUSHDigest[i] = byte(i * 3)
	}
	return s
}

func TestSuperblock_MarshalRoundtrip(t *testing.T) {
	s := makeSB()
	buf, err := s.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if len(buf) != SuperblockSize {
		t.Fatalf("marshalled size = %d, want %d", len(buf), SuperblockSize)
	}
	got, err := UnmarshalSuperblock(buf)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Version != s.Version || got.PoolID != s.PoolID || got.Role != s.Role {
		t.Errorf("field mismatch: got %+v, want %+v", got, s)
	}
	if got.MetaVolumeName != s.MetaVolumeName {
		t.Errorf("meta vol name: got %q want %q", got.MetaVolumeName, s.MetaVolumeName)
	}
	if got.MetaVolumeRootChunk != s.MetaVolumeRootChunk {
		t.Errorf("meta vol root: got %q want %q", got.MetaVolumeRootChunk, s.MetaVolumeRootChunk)
	}
	if got.MetaVolumeVersion != 42 {
		t.Errorf("meta vol version: got %d want 42", got.MetaVolumeVersion)
	}
	if got.DiskUUID != s.DiskUUID {
		t.Errorf("disk UUID mismatch")
	}
	if got.CRUSHDigest != s.CRUSHDigest {
		t.Errorf("CRUSH digest mismatch")
	}
}

func TestSuperblock_BadMagic(t *testing.T) {
	s := makeSB()
	buf, _ := s.Marshal()
	buf[0] ^= 0xFF
	// Because we changed bytes before the CRC the CRC will also be wrong,
	// but the magic check fires first.
	_, err := UnmarshalSuperblock(buf)
	if !errors.Is(err, ErrSuperblockBadMagic) {
		t.Fatalf("want ErrSuperblockBadMagic, got %v", err)
	}
}

func TestSuperblock_BadCRC(t *testing.T) {
	s := makeSB()
	buf, _ := s.Marshal()
	// Flip a byte after magic but before the CRC field.
	buf[sbOffPoolID] ^= 0x01
	_, err := UnmarshalSuperblock(buf)
	if !errors.Is(err, ErrSuperblockBadCRC) {
		t.Fatalf("want ErrSuperblockBadCRC, got %v", err)
	}
}

func TestSuperblock_FieldTooLong(t *testing.T) {
	s := makeSB()
	s.PoolID = string(make([]byte, sbFieldPoolIDLen+1))
	if _, err := s.Marshal(); !errors.Is(err, ErrFieldTooLong) {
		t.Fatalf("want ErrFieldTooLong, got %v", err)
	}
}

func TestSuperblock_WriteReadFile(t *testing.T) {
	// Simulate a "device" with a regular file large enough to hold the SB.
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-disk")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(64 * 1024); err != nil {
		t.Fatal(err)
	}
	f.Close()

	s := makeSB()
	if err := WriteSuperblock(path, s); err != nil {
		t.Fatalf("WriteSuperblock: %v", err)
	}
	got, err := ReadSuperblock(path)
	if err != nil {
		t.Fatalf("ReadSuperblock: %v", err)
	}
	if got.PoolID != s.PoolID {
		t.Errorf("roundtrip mismatch: PoolID got %q want %q", got.PoolID, s.PoolID)
	}
	if got.MetaVolumeRootChunk != s.MetaVolumeRootChunk {
		t.Errorf("roundtrip mismatch: meta root got %q want %q", got.MetaVolumeRootChunk, s.MetaVolumeRootChunk)
	}
	if got.UpdatedUnixNanos == 0 {
		t.Error("UpdatedUnixNanos should be stamped on write")
	}
}

func TestSuperblock_ReadMissingDevice(t *testing.T) {
	if _, err := ReadSuperblock("/nonexistent/novanas/disk"); err == nil {
		t.Error("reading nonexistent device should error")
	}
}
