package csi

import (
	"context"
	"strings"
	"testing"

	csipb "github.com/container-storage-interface/spec/lib/go/csi"
)

func TestNodeGetInfo(t *testing.T) {
	d := newTestDriver(newFakeClient(), newFakeMounter())
	resp, err := (&NodeService{d: d}).NodeGetInfo(context.Background(), &csipb.NodeGetInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.NodeId != "test-node" {
		t.Fatalf("unexpected NodeId: %s", resp.NodeId)
	}
}

func TestNodeGetCapabilities(t *testing.T) {
	d := newTestDriver(newFakeClient(), newFakeMounter())
	resp, err := (&NodeService{d: d}).NodeGetCapabilities(context.Background(), &csipb.NodeGetCapabilitiesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[csipb.NodeServiceCapability_RPC_Type]bool{}
	for _, c := range resp.Capabilities {
		got[c.GetRpc().Type] = true
	}
	if !got[csipb.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME] {
		t.Errorf("missing STAGE_UNSTAGE_VOLUME")
	}
	if !got[csipb.NodeServiceCapability_RPC_EXPAND_VOLUME] {
		t.Errorf("missing EXPAND_VOLUME")
	}
}

func TestNodePublish_Filesystem(t *testing.T) {
	c := newFakeClient()
	c.datasets["tank/csi/pvc-fs"] = &Dataset{Name: "tank/csi/pvc-fs", Type: "filesystem", Mountpoint: "/tank/csi/pvc-fs"}
	m := newFakeMounter()
	d := newTestDriver(c, m)
	ns := &NodeService{d: d}
	_, err := ns.NodePublishVolume(context.Background(), &csipb.NodePublishVolumeRequest{
		VolumeId:         "tank/csi/pvc-fs",
		TargetPath:       "/var/lib/kubelet/.../target",
		VolumeCapability: mountCap("ext4"),
		VolumeContext:    map[string]string{ctxVolumeKind: "filesystem"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.BindCalls) != 1 {
		t.Fatalf("expected 1 bind call, got %d (%v)", len(m.BindCalls), m.BindCalls)
	}
	if !strings.HasPrefix(m.BindCalls[0], "/tank/csi/pvc-fs->") {
		t.Fatalf("unexpected bind source: %s", m.BindCalls[0])
	}
}

func TestNodePublish_Block(t *testing.T) {
	c := newFakeClient()
	m := newFakeMounter()
	d := newTestDriver(c, m)
	ns := &NodeService{d: d}
	_, err := ns.NodePublishVolume(context.Background(), &csipb.NodePublishVolumeRequest{
		VolumeId:         "tank/csi/pvc-blk",
		TargetPath:       "/dev/blocktarget",
		VolumeCapability: blockCap(),
		VolumeContext:    map[string]string{ctxVolumeKind: "volume"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.BindCalls) != 1 {
		t.Fatalf("expected 1 bind call, got %d", len(m.BindCalls))
	}
	if !strings.HasPrefix(m.BindCalls[0], "/dev/zvol/tank/csi/pvc-blk->") {
		t.Fatalf("unexpected bind source: %s", m.BindCalls[0])
	}
	if !m.files["/dev/blocktarget"] {
		t.Fatalf("expected target file ensured for raw block")
	}
}

func TestNodeStage_FsOnZvol_FormatsAndMounts(t *testing.T) {
	c := newFakeClient()
	m := newFakeMounter()
	d := newTestDriver(c, m)
	ns := &NodeService{d: d}
	_, err := ns.NodeStageVolume(context.Background(), &csipb.NodeStageVolumeRequest{
		VolumeId:          "tank/csi/pvc-blk",
		StagingTargetPath: "/var/lib/kubelet/staging",
		VolumeCapability:  mountCap("ext4"),
		VolumeContext:     map[string]string{ctxVolumeKind: "volume"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.MkfsCalls) != 1 || m.MkfsCalls[0] != "ext4:/dev/zvol/tank/csi/pvc-blk" {
		t.Fatalf("unexpected mkfs calls: %v", m.MkfsCalls)
	}
	if len(m.BindCalls) != 1 {
		t.Fatalf("expected 1 stage bind, got %d", len(m.BindCalls))
	}
}

func TestNodePublish_NFS(t *testing.T) {
	c := newFakeClient()
	m := newFakeMounter()
	d := newTestDriver(c, m)
	ns := &NodeService{d: d}
	_, err := ns.NodePublishVolume(context.Background(), &csipb.NodePublishVolumeRequest{
		VolumeId:         "tank/csi-nfs/pvc-nfs",
		TargetPath:       "/var/lib/kubelet/.../target",
		VolumeCapability: mountCap(""),
		VolumeContext: map[string]string{
			ctxVolumeKind:   volumeKindNFS,
			ctxNFSServer:    "nas",
			ctxNFSPath:      "/tank/csi-nfs/pvc-nfs",
			ctxMountOptions: DefaultNFSMountOptions,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.NFSCalls) != 1 {
		t.Fatalf("expected 1 NFS mount, got %d (%v)", len(m.NFSCalls), m.NFSCalls)
	}
	if !strings.Contains(m.NFSCalls[0], "nas:/tank/csi-nfs/pvc-nfs->") {
		t.Fatalf("unexpected NFS call: %s", m.NFSCalls[0])
	}
	if !strings.Contains(m.NFSCalls[0], "sec=krb5p") {
		t.Fatalf("expected sec=krb5p in mount opts, got %s", m.NFSCalls[0])
	}
	if len(m.BindCalls) != 0 {
		t.Fatalf("NFS publish should not bind-mount, got %v", m.BindCalls)
	}
}

func TestNodeStage_NFS_NoOp(t *testing.T) {
	d := newTestDriver(newFakeClient(), newFakeMounter())
	ns := &NodeService{d: d}
	if _, err := ns.NodeStageVolume(context.Background(), &csipb.NodeStageVolumeRequest{
		VolumeId:          "tank/csi-nfs/pvc-nfs",
		StagingTargetPath: "/staging",
		VolumeCapability:  mountCap(""),
		VolumeContext:     map[string]string{ctxVolumeKind: volumeKindNFS},
	}); err != nil {
		t.Fatalf("NFS stage should be no-op: %v", err)
	}
}

func TestNodeUnpublish(t *testing.T) {
	m := newFakeMounter()
	m.mounted["/target"] = true
	d := newTestDriver(newFakeClient(), m)
	ns := &NodeService{d: d}
	_, err := ns.NodeUnpublishVolume(context.Background(), &csipb.NodeUnpublishVolumeRequest{TargetPath: "/target"})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.UnmountCalls) != 1 || m.UnmountCalls[0] != "/target" {
		t.Fatalf("unexpected umount calls: %v", m.UnmountCalls)
	}
}
