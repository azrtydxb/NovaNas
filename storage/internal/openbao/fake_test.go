package openbao

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"
)

func TestFakeTransit_WrapUnwrap(t *testing.T) {
	f := NewFakeTransit()
	ctx := context.Background()

	dk := make([]byte, 32)
	_, _ = rand.Read(dk)

	wrapped, version, err := f.WrapDK(ctx, "novanas/chunk-master", dk)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	if version == 0 {
		t.Error("version should be non-zero")
	}
	got, err := f.UnwrapDK(ctx, "novanas/chunk-master", wrapped)
	if err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	if !bytes.Equal(got, dk) {
		t.Error("roundtrip mismatch")
	}
}

// TestFakeTransit_RotationPreservesOldUnwrap: after rotating the
// master key, old wrapped blobs must still be unwrappable (they carry
// their version); new wraps use the new version.
func TestFakeTransit_RotationPreservesOldUnwrap(t *testing.T) {
	f := NewFakeTransit()
	ctx := context.Background()
	name := "novanas/chunk-master"

	dk1 := bytes.Repeat([]byte{0xA5}, 32)
	oldWrapped, v1, err := f.WrapDK(ctx, name, dk1)
	if err != nil {
		t.Fatal(err)
	}

	if err := f.RotateMasterKey(ctx, name); err != nil {
		t.Fatal(err)
	}

	dk2 := bytes.Repeat([]byte{0x5A}, 32)
	newWrapped, v2, err := f.WrapDK(ctx, name, dk2)
	if err != nil {
		t.Fatal(err)
	}
	if v2 != v1+1 {
		t.Errorf("expected version bump, got v1=%d v2=%d", v1, v2)
	}

	// Old wrap still works.
	got1, err := f.UnwrapDK(ctx, name, oldWrapped)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got1, dk1) {
		t.Error("old wrap mismatch after rotation")
	}
	// New wrap also works.
	got2, err := f.UnwrapDK(ctx, name, newWrapped)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got2, dk2) {
		t.Error("new wrap mismatch")
	}

	cfg, err := f.ReadConfig(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LatestVersion != v2 {
		t.Errorf("config latest=%d want %d", cfg.LatestVersion, v2)
	}
}
