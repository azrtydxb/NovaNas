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

func TestVolumeKeyManager_DestroyVolume_CryptographicErase(t *testing.T) {
	ctx := context.Background()
	fake := openbao.NewFakeTransit()
	keyName := VolumeTransitKeyName("vol-erase")
	m := NewVolumeKeyManager(fake, keyName)

	// Provision: wrapped DK materialises in the Fake's keys map under keyName.
	wrapped, version, err := m.ProvisionVolume(ctx, "vol-erase")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if _, err := fake.ReadConfig(ctx, keyName); err != nil {
		t.Fatalf("expected key to exist after provision: %v", err)
	}

	// Destroy.
	if err := m.DestroyVolume(ctx, "vol-erase", keyName); err != nil {
		t.Fatalf("DestroyVolume: %v", err)
	}
	// Cache is evicted.
	if _, ok := m.Get("vol-erase"); ok {
		t.Fatal("cache entry should be evicted after DestroyVolume")
	}
	// Mount now fails because the Transit key is gone.
	if err := m.Mount(ctx, "vol-erase", wrapped, version); err == nil {
		t.Fatalf("expected Mount to fail after cryptographic erase")
	}
	// Double-destroy surfaces the backend error so the caller can react.
	if err := m.DestroyVolume(ctx, "vol-erase", keyName); err == nil {
		t.Fatalf("expected error when destroying an already-destroyed key")
	}
}

func TestVolumeKeyManager_DestroyVolume_RequiresKeyName(t *testing.T) {
	m := NewVolumeKeyManager(openbao.NewFakeTransit(), "mk")
	if err := m.DestroyVolume(context.Background(), "vol-x", ""); err == nil {
		t.Fatal("expected error when keyName is empty")
	}
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
