package csi

import (
	"context"
	"testing"

	csipb "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func mountCap(fs string) *csipb.VolumeCapability {
	return &csipb.VolumeCapability{
		AccessMode: &csipb.VolumeCapability_AccessMode{
			Mode: csipb.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
		AccessType: &csipb.VolumeCapability_Mount{Mount: &csipb.VolumeCapability_MountVolume{FsType: fs}},
	}
}
func blockCap() *csipb.VolumeCapability {
	return &csipb.VolumeCapability{
		AccessMode: &csipb.VolumeCapability_AccessMode{Mode: csipb.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
		AccessType: &csipb.VolumeCapability_Block{Block: &csipb.VolumeCapability_BlockVolume{}},
	}
}

func TestControllerGetCapabilities(t *testing.T) {
	d := newTestDriver(newFakeClient(), newFakeMounter())
	resp, err := (&ControllerService{d: d}).ControllerGetCapabilities(context.Background(), &csipb.ControllerGetCapabilitiesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[csipb.ControllerServiceCapability_RPC_Type]bool{}
	for _, c := range resp.Capabilities {
		got[c.GetRpc().Type] = true
	}
	for _, want := range []csipb.ControllerServiceCapability_RPC_Type{
		csipb.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csipb.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csipb.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csipb.ControllerServiceCapability_RPC_GET_VOLUME,
	} {
		if !got[want] {
			t.Errorf("missing capability %v", want)
		}
	}
}

func TestCreateVolume_Filesystem(t *testing.T) {
	c := newFakeClient()
	d := newTestDriver(c, newFakeMounter())
	cs := &ControllerService{d: d}
	resp, err := cs.CreateVolume(context.Background(), &csipb.CreateVolumeRequest{
		Name:               "pvc-fs",
		CapacityRange:      &csipb.CapacityRange{RequiredBytes: 4 << 20},
		VolumeCapabilities: []*csipb.VolumeCapability{mountCap("ext4")},
		Parameters:         map[string]string{"pool": "tank"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Volume.VolumeId != "tank/csi/pvc-fs" {
		t.Fatalf("unexpected VolumeId: %s", resp.Volume.VolumeId)
	}
	if c.LastCreate.Type != "filesystem" {
		t.Fatalf("expected filesystem, got %s", c.LastCreate.Type)
	}
	if c.LastCreate.Parent != "tank/csi" || c.LastCreate.Name != "pvc-fs" {
		t.Fatalf("unexpected create spec: %+v", c.LastCreate)
	}
	if c.LastCreate.Properties["quota"] == "" || c.LastCreate.Properties["refquota"] == "" {
		t.Fatalf("expected quota/refquota set, got %+v", c.LastCreate.Properties)
	}
	if c.LastCreate.Properties["compression"] != "lz4" {
		t.Fatalf("expected compression=lz4, got %q", c.LastCreate.Properties["compression"])
	}
}

func TestCreateVolume_Block(t *testing.T) {
	c := newFakeClient()
	d := newTestDriver(c, newFakeMounter())
	cs := &ControllerService{d: d}
	_, err := cs.CreateVolume(context.Background(), &csipb.CreateVolumeRequest{
		Name:               "pvc-blk",
		CapacityRange:      &csipb.CapacityRange{RequiredBytes: 1 << 30},
		VolumeCapabilities: []*csipb.VolumeCapability{blockCap()},
		Parameters:         map[string]string{"pool": "tank", "volblocksize": "16K"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.LastCreate.Type != "volume" {
		t.Fatalf("expected volume, got %s", c.LastCreate.Type)
	}
	if c.LastCreate.VolumeSizeBytes != 1<<30 {
		t.Fatalf("expected size 1GiB, got %d", c.LastCreate.VolumeSizeBytes)
	}
	if c.LastCreate.Properties["volblocksize"] != "16K" {
		t.Fatalf("expected volblocksize=16K, got %q", c.LastCreate.Properties["volblocksize"])
	}
}

func TestCreateVolume_Idempotent(t *testing.T) {
	c := newFakeClient()
	d := newTestDriver(c, newFakeMounter())
	cs := &ControllerService{d: d}
	req := &csipb.CreateVolumeRequest{
		Name:               "pvc-x",
		CapacityRange:      &csipb.CapacityRange{RequiredBytes: 4 << 20},
		VolumeCapabilities: []*csipb.VolumeCapability{mountCap("ext4")},
		Parameters:         map[string]string{"pool": "tank"},
	}
	r1, err := cs.CreateVolume(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := cs.CreateVolume(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Volume.VolumeId != r2.Volume.VolumeId {
		t.Fatalf("VolumeId changed across idempotent calls: %s vs %s", r1.Volume.VolumeId, r2.Volume.VolumeId)
	}
	if c.CreateCount != 1 {
		t.Fatalf("expected exactly one CreateDataset call, got %d", c.CreateCount)
	}
}

func TestCreateVolume_MixedCapsRejected(t *testing.T) {
	c := newFakeClient()
	d := newTestDriver(c, newFakeMounter())
	cs := &ControllerService{d: d}
	_, err := cs.CreateVolume(context.Background(), &csipb.CreateVolumeRequest{
		Name:               "pvc-mix",
		VolumeCapabilities: []*csipb.VolumeCapability{mountCap("ext4"), blockCap()},
		Parameters:         map[string]string{"pool": "tank"},
	})
	if err == nil {
		t.Fatal("expected error for mixed caps")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestDeleteVolume_NotFoundIsOK(t *testing.T) {
	c := newFakeClient()
	d := newTestDriver(c, newFakeMounter())
	cs := &ControllerService{d: d}
	_, err := cs.DeleteVolume(context.Background(), &csipb.DeleteVolumeRequest{VolumeId: "tank/csi/missing"})
	if err != nil {
		t.Fatalf("idempotent delete should not error: %v", err)
	}
}

func TestExpandVolume_Filesystem(t *testing.T) {
	c := newFakeClient()
	c.datasets["tank/csi/pvc-fs"] = &Dataset{Name: "tank/csi/pvc-fs", Type: "filesystem", Quota: 1 << 20}
	d := newTestDriver(c, newFakeMounter())
	cs := &ControllerService{d: d}
	resp, err := cs.ControllerExpandVolume(context.Background(), &csipb.ControllerExpandVolumeRequest{
		VolumeId:      "tank/csi/pvc-fs",
		CapacityRange: &csipb.CapacityRange{RequiredBytes: 4 << 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.NodeExpansionRequired {
		t.Fatalf("filesystem expand should not require node expansion")
	}
	if c.LastSetProps["quota"] == "" || c.LastSetProps["refquota"] == "" {
		t.Fatalf("expected quota/refquota set, got %+v", c.LastSetProps)
	}
}

func TestExpandVolume_Zvol(t *testing.T) {
	c := newFakeClient()
	c.datasets["tank/csi/pvc-blk"] = &Dataset{Name: "tank/csi/pvc-blk", Type: "volume", Volsize: 1 << 30}
	d := newTestDriver(c, newFakeMounter())
	cs := &ControllerService{d: d}
	resp, err := cs.ControllerExpandVolume(context.Background(), &csipb.ControllerExpandVolumeRequest{
		VolumeId:         "tank/csi/pvc-blk",
		CapacityRange:    &csipb.CapacityRange{RequiredBytes: 2 << 30},
		VolumeCapability: mountCap("ext4"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.NodeExpansionRequired {
		t.Fatalf("zvol with fs expand should require node expansion")
	}
	if c.LastSetProps["volsize"] == "" {
		t.Fatalf("expected volsize set, got %+v", c.LastSetProps)
	}
}

func TestSnapshot_CreateAndDelete(t *testing.T) {
	c := newFakeClient()
	c.datasets["tank/csi/pvc-fs"] = &Dataset{Name: "tank/csi/pvc-fs", Type: "filesystem"}
	d := newTestDriver(c, newFakeMounter())
	cs := &ControllerService{d: d}
	resp, err := cs.CreateSnapshot(context.Background(), &csipb.CreateSnapshotRequest{
		Name:           "snap1",
		SourceVolumeId: "tank/csi/pvc-fs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Snapshot.SnapshotId != "tank/csi/pvc-fs@snap1" {
		t.Fatalf("unexpected SnapshotId: %s", resp.Snapshot.SnapshotId)
	}
	if _, err := cs.DeleteSnapshot(context.Background(), &csipb.DeleteSnapshotRequest{SnapshotId: resp.Snapshot.SnapshotId}); err != nil {
		t.Fatal(err)
	}
	// Idempotent re-delete.
	if _, err := cs.DeleteSnapshot(context.Background(), &csipb.DeleteSnapshotRequest{SnapshotId: resp.Snapshot.SnapshotId}); err != nil {
		t.Fatalf("idempotent snapshot delete should not error: %v", err)
	}
}

func TestValidateVolumeCapabilities(t *testing.T) {
	d := newTestDriver(newFakeClient(), newFakeMounter())
	cs := &ControllerService{d: d}
	resp, err := cs.ValidateVolumeCapabilities(context.Background(), &csipb.ValidateVolumeCapabilitiesRequest{
		VolumeId:           "tank/csi/pvc",
		VolumeCapabilities: []*csipb.VolumeCapability{mountCap("ext4")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Confirmed == nil {
		t.Fatalf("expected confirmed, got %+v", resp)
	}
}
