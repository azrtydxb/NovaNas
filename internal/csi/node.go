package csi

import (
	"context"

	csipb "github.com/container-storage-interface/spec/lib/go/csi"
)

// NodeService implements csi.NodeServer.
type NodeService struct {
	csipb.UnimplementedNodeServer
	d *Driver
}

// NodeGetInfo returns the node identifier (no topology in v1).
func (s *NodeService) NodeGetInfo(ctx context.Context, _ *csipb.NodeGetInfoRequest) (*csipb.NodeGetInfoResponse, error) {
	return &csipb.NodeGetInfoResponse{NodeId: s.d.cfg.NodeID}, nil
}

// NodeGetCapabilities advertises STAGE_UNSTAGE_VOLUME and EXPAND_VOLUME.
func (s *NodeService) NodeGetCapabilities(ctx context.Context, _ *csipb.NodeGetCapabilitiesRequest) (*csipb.NodeGetCapabilitiesResponse, error) {
	caps := []csipb.NodeServiceCapability_RPC_Type{
		csipb.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csipb.NodeServiceCapability_RPC_EXPAND_VOLUME,
	}
	out := make([]*csipb.NodeServiceCapability, 0, len(caps))
	for _, c := range caps {
		out = append(out, &csipb.NodeServiceCapability{
			Type: &csipb.NodeServiceCapability_Rpc{
				Rpc: &csipb.NodeServiceCapability_RPC{Type: c},
			},
		})
	}
	return &csipb.NodeGetCapabilitiesResponse{Capabilities: out}, nil
}

// NodeStageVolume stages a volume on the node. For ZFS filesystem datasets
// staging is a no-op (the dataset is auto-mounted by ZFS at its mountpoint).
// For zvols accessed as raw block, also a no-op (kubelet handles the bind).
// For zvols formatted as a filesystem (Mount access type), we mkfs if needed
// and bind-mount the device into the staging path.
func (s *NodeService) NodeStageVolume(ctx context.Context, req *csipb.NodeStageVolumeRequest) (*csipb.NodeStageVolumeResponse, error) {
	id, err := ParseVolumeID(req.GetVolumeId())
	if err != nil {
		return nil, errInvalid("VolumeId: %v", err)
	}
	cap := req.GetVolumeCapability()
	if cap == nil {
		return nil, errInvalid("VolumeCapability is required")
	}
	kind := s.kindFromContext(req.GetVolumeContext())

	// For block-mode block volumes, nothing to stage.
	if _, isBlock := cap.GetAccessType().(*csipb.VolumeCapability_Block); isBlock {
		return &csipb.NodeStageVolumeResponse{}, nil
	}

	// Filesystem dataset: nothing to stage. The dataset is mounted by ZFS.
	if kind == "filesystem" {
		return &csipb.NodeStageVolumeResponse{}, nil
	}

	// NFS volumes: kubelet calls NodePublish directly; staging is a no-op.
	if kind == volumeKindNFS {
		return &csipb.NodeStageVolumeResponse{}, nil
	}

	// fs-on-zvol: mkfs (if needed) and mount to staging path.
	device := ZvolDevicePath(id)
	staging := req.GetStagingTargetPath()
	fsType := mountFsType(cap, "ext4")

	formatted, existing, err := s.d.mounter.IsFormatted(device)
	if err != nil {
		return nil, errInternal("blkid %s: %v", device, err)
	}
	if !formatted {
		if err := s.d.mounter.Mkfs(device, fsType); err != nil {
			return nil, errInternal("mkfs.%s %s: %v", fsType, device, err)
		}
	} else if existing != fsType {
		return nil, errInternal("device %s already has fs %q, requested %q", device, existing, fsType)
	}

	if err := s.d.mounter.EnsureDir(staging); err != nil {
		return nil, errInternal("mkdir staging: %v", err)
	}
	mounted, err := s.d.mounter.IsMounted(staging)
	if err != nil {
		return nil, errInternal("check staging mount: %v", err)
	}
	if !mounted {
		if err := s.d.mounter.BindMount(device, staging, false); err != nil {
			return nil, errInternal("stage mount: %v", err)
		}
	}
	return &csipb.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume undoes NodeStageVolume.
func (s *NodeService) NodeUnstageVolume(ctx context.Context, req *csipb.NodeUnstageVolumeRequest) (*csipb.NodeUnstageVolumeResponse, error) {
	if req.GetStagingTargetPath() == "" {
		return nil, errInvalid("StagingTargetPath is required")
	}
	if err := s.d.mounter.Unmount(req.GetStagingTargetPath()); err != nil {
		return nil, errInternal("unmount staging: %v", err)
	}
	return &csipb.NodeUnstageVolumeResponse{}, nil
}

// NodePublishVolume bind-mounts the volume to the target path. For
// kind=nfs volumes it performs a mount.nfs4 instead of a bind, so the
// kernel NFS client (with kerberos via host krb5.keytab) backs the mount
// and NFSv4 ACLs are enforced server-side.
func (s *NodeService) NodePublishVolume(ctx context.Context, req *csipb.NodePublishVolumeRequest) (*csipb.NodePublishVolumeResponse, error) {
	id, err := ParseVolumeID(req.GetVolumeId())
	if err != nil {
		return nil, errInvalid("VolumeId: %v", err)
	}
	cap := req.GetVolumeCapability()
	if cap == nil {
		return nil, errInvalid("VolumeCapability is required")
	}
	target := req.GetTargetPath()
	if target == "" {
		return nil, errInvalid("TargetPath is required")
	}
	readonly := req.GetReadonly()
	kind := s.kindFromContext(req.GetVolumeContext())

	// kind=nfs has its own publish path (mount.nfs4).
	if kind == volumeKindNFS {
		return s.publishNFS(req, target, readonly)
	}

	var source string
	isBlock := false
	if _, ok := cap.GetAccessType().(*csipb.VolumeCapability_Block); ok {
		// Raw block: bind /dev/zvol device directly.
		source = s.d.hostPath(ZvolDevicePath(id))
		isBlock = true
	} else if kind == "filesystem" {
		// Filesystem dataset: mount the dataset's directory mountpoint.
		ds, err := s.d.client.GetDataset(ctx, id.Full)
		if err != nil {
			return nil, errInternal("get dataset: %v", err)
		}
		if ds.Mountpoint == "" {
			return nil, errInternal("dataset %s has no mountpoint", id.Full)
		}
		source = s.d.hostPath(ds.Mountpoint)
	} else {
		// fs-on-zvol: source is the staging path established earlier.
		source = req.GetStagingTargetPath()
		if source == "" {
			return nil, errInvalid("StagingTargetPath is required for fs-on-zvol publish")
		}
	}

	// Prepare target.
	if isBlock {
		if err := s.d.mounter.EnsureFile(target); err != nil {
			return nil, errInternal("ensure target file: %v", err)
		}
	} else {
		if err := s.d.mounter.EnsureDir(target); err != nil {
			return nil, errInternal("ensure target dir: %v", err)
		}
	}
	mounted, err := s.d.mounter.IsMounted(target)
	if err != nil {
		return nil, errInternal("check target mount: %v", err)
	}
	if mounted {
		// Idempotent — assume previous publish.
		return &csipb.NodePublishVolumeResponse{}, nil
	}
	if err := s.d.mounter.BindMount(source, target, readonly); err != nil {
		return nil, errInternal("bind-mount %s -> %s: %v", source, target, err)
	}
	return &csipb.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmounts the target path.
func (s *NodeService) NodeUnpublishVolume(ctx context.Context, req *csipb.NodeUnpublishVolumeRequest) (*csipb.NodeUnpublishVolumeResponse, error) {
	if req.GetTargetPath() == "" {
		return nil, errInvalid("TargetPath is required")
	}
	if err := s.d.mounter.Unmount(req.GetTargetPath()); err != nil {
		return nil, errInternal("unmount target: %v", err)
	}
	return &csipb.NodeUnpublishVolumeResponse{}, nil
}

// NodeExpandVolume grows the filesystem on a zvol after the controller has
// resized volsize. For kind=nfs the controller-side dataset quota change
// is visible to clients automatically; this is a no-op.
func (s *NodeService) NodeExpandVolume(ctx context.Context, req *csipb.NodeExpandVolumeRequest) (*csipb.NodeExpandVolumeResponse, error) {
	id, err := ParseVolumeID(req.GetVolumeId())
	if err != nil {
		return nil, errInvalid("VolumeId: %v", err)
	}
	// kind=nfs has no underlying block device. The existing IsFormatted-
	// returns-empty path below handles this gracefully (CapacityBytes is
	// echoed back without growfs); no NFS-specific shortcut is needed.
	device := ZvolDevicePath(id)
	target := req.GetVolumePath()
	if target == "" {
		return nil, errInvalid("VolumePath is required")
	}
	// Best-effort fs detection via blkid.
	_, fsType, err := s.d.mounter.IsFormatted(device)
	if err != nil {
		return nil, errInternal("detect fs: %v", err)
	}
	if fsType == "" {
		// Filesystem dataset (no underlying block dev to grow). Nothing to do.
		return &csipb.NodeExpandVolumeResponse{
			CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
		}, nil
	}
	if err := s.d.mounter.GrowFS(target, device, fsType); err != nil {
		return nil, errInternal("growfs: %v", err)
	}
	return &csipb.NodeExpandVolumeResponse{
		CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
	}, nil
}

// publishNFS handles the NodePublishVolume path for NFS-mode volumes. It
// reads the server / export path / mount options from VolumeContext (the
// controller stamped them at CreateVolume time) and shells out to
// mount.nfs4. The kernel does the krb5p negotiation via the host's
// /etc/krb5.keytab, which the DaemonSet mounts read-only.
func (s *NodeService) publishNFS(req *csipb.NodePublishVolumeRequest, target string, readonly bool) (*csipb.NodePublishVolumeResponse, error) {
	vc := req.GetVolumeContext()
	server := vc[ctxNFSServer]
	exportPath := vc[ctxNFSPath]
	mountOpts := vc[ctxMountOptions]
	if server == "" || exportPath == "" {
		return nil, errInvalid("VolumeContext is missing nfsServer or nfsPath for NFS volume")
	}
	if err := s.d.mounter.EnsureDir(target); err != nil {
		return nil, errInternal("ensure target dir: %v", err)
	}
	mounted, err := s.d.mounter.IsMounted(target)
	if err != nil {
		return nil, errInternal("check target mount: %v", err)
	}
	if mounted {
		return &csipb.NodePublishVolumeResponse{}, nil
	}
	if err := s.d.mounter.NFSMount(server, exportPath, target, NFSMountOpts{
		Options:  mountOpts,
		ReadOnly: readonly,
	}); err != nil {
		return nil, errInternal("mount.nfs4 %s:%s -> %s: %v", server, exportPath, target, err)
	}
	return &csipb.NodePublishVolumeResponse{}, nil
}

// kindFromContext reads ctxVolumeKind out of VolumeContext, defaulting to
// "filesystem" when absent.
func (s *NodeService) kindFromContext(c map[string]string) string {
	if c == nil {
		return "filesystem"
	}
	if v := c[ctxVolumeKind]; v != "" {
		return v
	}
	return "filesystem"
}

// mountFsType returns the requested fsType from a Mount capability, falling
// back to def when unset.
func mountFsType(cap *csipb.VolumeCapability, def string) string {
	if m, ok := cap.GetAccessType().(*csipb.VolumeCapability_Mount); ok && m.Mount != nil {
		if m.Mount.FsType != "" {
			return m.Mount.FsType
		}
	}
	return def
}
