package crypto

import (
	"bytes"
	"context"
	"testing"

	"github.com/azrtydxb/novanas/storage/internal/openbao"
)

func TestVolumeKeyManager_ProvisionMountUnmount(t *testing.T) {
	ctx := context.Background()
	fake := openbao.NewFakeTransit()
	m := NewVolumeKeyManager(fake, "novanas/chunk-master")

	wrapped, version, err := m.ProvisionVolume(ctx, "vol-1")
	if err != nil {
		t.Fatal(err)
	}
	if version == 0 {
		t.Error("expected non-zero version")
	}

	dk, ok := m.Get("vol-1")
	if !ok {
		t.Fatal("provisioned volume should be cached")
	}
	raw1, _ := dk.Bytes()

	// Simulate restart: drop cache and mount from wrapped.
	m.Unmount("vol-1")
	if _, ok := m.Get("vol-1"); ok {
		t.Fatal("should be evicted")
	}

	if err := m.Mount(ctx, "vol-1", wrapped, version); err != nil {
		t.Fatal(err)
	}
	dk2, ok := m.Get("vol-1")
	if !ok {
		t.Fatal("should be cached after mount")
	}
	raw2, _ := dk2.Bytes()

	if !bytes.Equal(raw1, raw2) {
		t.Error("unwrapped DK differs from originally provisioned DK")
	}
	ZeroBytes(raw1)
	ZeroBytes(raw2)
}

func TestVolumeKeyManager_RotationSurvives(t *testing.T) {
	ctx := context.Background()
	fake := openbao.NewFakeTransit()
	m := NewVolumeKeyManager(fake, "mk")

	wrapped, _, err := m.ProvisionVolume(ctx, "vol-1")
	if err != nil {
		t.Fatal(err)
	}
	// Rotate master; existing wrapped blob must still unwrap.
	if err := fake.RotateMasterKey(ctx, "mk"); err != nil {
		t.Fatal(err)
	}
	m.Unmount("vol-1")
	if err := m.Mount(ctx, "vol-1", wrapped, 1); err != nil {
		t.Fatalf("mount after rotation: %v", err)
	}
}
