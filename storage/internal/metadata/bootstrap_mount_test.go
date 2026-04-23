package metadata

import (
	"context"
	"errors"
	"testing"
)

func TestNoopMounter_ReturnsMountNotSupported(t *testing.T) {
	m := NoopMetadataVolumeMounter{}
	_, err := m.ExportMetadataVolume(context.Background(), VolumeLocator{Name: "v"})
	if !errors.Is(err, ErrMountNotSupported) {
		t.Fatalf("want ErrMountNotSupported, got %v", err)
	}
	if err := m.ReleaseMetadataVolume(context.Background(), VolumeLocator{}); err != nil {
		t.Fatalf("release: unexpected err %v", err)
	}
}

func TestDataplaneNBDMounter_DelegatesExport(t *testing.T) {
	called := false
	m := &DataplaneNBDMounter{
		Export: func(_ context.Context, loc VolumeLocator) (string, error) {
			called = true
			if loc.RootChunk != "root-abc" {
				t.Errorf("wrong locator: %+v", loc)
			}
			return "/dev/loop0", nil
		},
	}
	dev, err := m.ExportMetadataVolume(context.Background(), VolumeLocator{Name: "meta", RootChunk: "root-abc", Version: 3})
	if err != nil {
		t.Fatal(err)
	}
	if dev != "/dev/loop0" {
		t.Fatalf("got device %q", dev)
	}
	if !called {
		t.Fatal("exporter was not called")
	}
}

func TestDataplaneNBDMounter_NoExporterReturnsMountNotSupported(t *testing.T) {
	m := &DataplaneNBDMounter{}
	_, err := m.ExportMetadataVolume(context.Background(), VolumeLocator{})
	if !errors.Is(err, ErrMountNotSupported) {
		t.Fatalf("want ErrMountNotSupported, got %v", err)
	}
}
