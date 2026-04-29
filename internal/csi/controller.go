package csi

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	csipb "github.com/container-storage-interface/spec/lib/go/csi"
)

// ControllerService implements csi.ControllerServer.
type ControllerService struct {
	csipb.UnimplementedControllerServer
	d *Driver
}

// minimum capacity if the request specifies no range (1 GiB).
const defaultCapacityBytes int64 = 1 << 30

// MiB rounding for filesystem datasets to keep ZFS happy.
const oneMiB int64 = 1 << 20

// Parameter keys accepted on the StorageClass.
const (
	paramPool           = "pool"
	paramParent         = "parent"
	paramCompression    = "compression"
	paramRecordsize     = "recordsize"
	paramVolblocksize   = "volblocksize"
	paramFsType         = "fsType"
	paramAccessProtocol = "accessProtocol" // "" (default = bind-mount) | "nfs"
	paramMountOptions   = "mountOptions"   // optional NFS mount options override
	paramNFSClients     = "nfsClients"     // comma-separated client allowlist for new NFS exports
	paramNFSSec         = "sec"            // export sec= flavor (informational; Agent C enforces krb5p in /etc/exports)
	// VolumeContext keys we set on the returned Volume.
	ctxFsType       = "fsType"
	ctxVolumeKind   = "volumeKind" // "filesystem" | "volume" | "nfs"
	ctxNFSServer    = "nfsServer"
	ctxNFSPath      = "nfsPath"
	ctxMountOptions = "mountOptions"

	// volumeKindNFS is the ctxVolumeKind value used for NFS-mode volumes.
	volumeKindNFS = "nfs"
)

// ControllerGetCapabilities advertises the operations we implement.
func (s *ControllerService) ControllerGetCapabilities(ctx context.Context, _ *csipb.ControllerGetCapabilitiesRequest) (*csipb.ControllerGetCapabilitiesResponse, error) {
	caps := []csipb.ControllerServiceCapability_RPC_Type{
		csipb.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csipb.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csipb.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csipb.ControllerServiceCapability_RPC_GET_VOLUME,
	}
	out := make([]*csipb.ControllerServiceCapability, 0, len(caps))
	for _, c := range caps {
		out = append(out, &csipb.ControllerServiceCapability{
			Type: &csipb.ControllerServiceCapability_Rpc{
				Rpc: &csipb.ControllerServiceCapability_RPC{Type: c},
			},
		})
	}
	return &csipb.ControllerGetCapabilitiesResponse{Capabilities: out}, nil
}

// volumeKind classifies the requested volume capabilities.
type volumeKind int

const (
	kindUnknown volumeKind = iota
	kindFilesystem
	kindBlock
	kindNFS
)

// classify inspects the capability list and StorageClass parameters. All
// caps must agree on access type. When parameters request the NFS protocol
// (accessProtocol=nfs), Mount caps are reclassified as kindNFS.
func classify(caps []*csipb.VolumeCapability, params map[string]string) (volumeKind, error) {
	if len(caps) == 0 {
		return kindUnknown, errInvalid("VolumeCapabilities is required")
	}
	var kind volumeKind
	for i, c := range caps {
		var k volumeKind
		switch c.GetAccessType().(type) {
		case *csipb.VolumeCapability_Mount:
			k = kindFilesystem
		case *csipb.VolumeCapability_Block:
			k = kindBlock
		default:
			return kindUnknown, errInvalid("VolumeCapabilities[%d]: access_type required", i)
		}
		if i == 0 {
			kind = k
			continue
		}
		if k != kind {
			return kindUnknown, errInvalid("mixed Block and Mount capabilities are not supported")
		}
	}
	// accessProtocol=nfs only applies to Mount-typed caps. Block volumes
	// cannot be served over NFS.
	if strings.EqualFold(params[paramAccessProtocol], "nfs") {
		if kind != kindFilesystem {
			return kindUnknown, errInvalid("accessProtocol=nfs requires Mount access type")
		}
		kind = kindNFS
	}
	return kind, nil
}

// CreateVolume provisions a ZFS dataset (filesystem) or zvol (block).
func (s *ControllerService) CreateVolume(ctx context.Context, req *csipb.CreateVolumeRequest) (*csipb.CreateVolumeResponse, error) {
	if req.GetName() == "" {
		return nil, errInvalid("Name is required")
	}
	params := req.GetParameters()
	kind, err := classify(req.GetVolumeCapabilities(), params)
	if err != nil {
		return nil, err
	}
	pool := firstNonEmpty(params[paramPool], s.d.cfg.DefaultPool)
	if pool == "" {
		return nil, errInvalid("StorageClass parameter 'pool' is required (no default-pool configured)")
	}
	parent := firstNonEmpty(params[paramParent], s.d.defaultParent(pool))
	leaf := req.GetName() // k8s sends e.g. pvc-<uuid>; use directly.
	full := EncodeVolumeID(parent, leaf)

	capacity := requestedCapacity(req.GetCapacityRange())
	// Round filesystem-style datasets to a MiB; zvols must align to volblocksize.
	if kind == kindFilesystem || kind == kindNFS {
		capacity = roundUp(capacity, oneMiB)
	}

	// kind=nfs has its own controller path: it creates a ProtocolShare
	// (dataset + NFS export) via the NovaNAS API rather than provisioning a
	// bare dataset. Idempotent on the share name (== leaf).
	if kind == kindNFS {
		return s.createNFSVolume(ctx, params, pool, parent, leaf, full, capacity)
	}

	// Build ZFS properties.
	props := map[string]string{}
	if c := params[paramCompression]; c != "" {
		props["compression"] = c
	} else {
		props["compression"] = "lz4"
	}
	switch kind {
	case kindFilesystem:
		if rs := params[paramRecordsize]; rs != "" {
			props["recordsize"] = rs
		}
		// Capacity is enforced via quota+refquota.
		props["quota"] = strconv.FormatInt(capacity, 10)
		props["refquota"] = strconv.FormatInt(capacity, 10)
	case kindBlock:
		if vbs := params[paramVolblocksize]; vbs != "" {
			props["volblocksize"] = vbs
		}
	}

	// Idempotency: GetDataset first.
	existing, getErr := s.d.client.GetDataset(ctx, full)
	if getErr != nil && !s.d.client.IsNotFound(getErr) {
		return nil, errInternal("get dataset %s: %v", full, getErr)
	}
	if existing != nil {
		// Verify type compatibility.
		wantType := zfsType(kind)
		if existing.Type != wantType {
			return nil, errAlreadyExists("dataset %s exists with type %q, want %q", full, existing.Type, wantType)
		}
		if !sizeCompatible(existing, kind, capacity) {
			return nil, errAlreadyExists("dataset %s exists with incompatible size", full)
		}
		return &csipb.CreateVolumeResponse{Volume: s.buildVolume(full, capacity, kind, params)}, nil
	}

	spec := CreateDatasetSpec{
		Parent:     parent,
		Name:       leaf,
		Type:       zfsType(kind),
		Properties: props,
	}
	if kind == kindBlock {
		spec.VolumeSizeBytes = capacity
	}
	job, err := s.d.client.CreateDataset(ctx, spec)
	if err != nil {
		return nil, errInternal("create dataset %s: %v", full, err)
	}
	if _, err := s.d.client.WaitJob(ctx, job.ID, JobPollInterval); err != nil {
		return nil, errInternal("wait create-dataset job: %v", err)
	}
	return &csipb.CreateVolumeResponse{Volume: s.buildVolume(full, capacity, kind, params)}, nil
}

func (s *ControllerService) buildVolume(volumeID string, capacity int64, kind volumeKind, params map[string]string) *csipb.Volume {
	ctx := map[string]string{
		ctxVolumeKind: zfsType(kind),
	}
	if kind == kindBlock {
		fs := params[paramFsType]
		if fs == "" {
			fs = "ext4"
		}
		ctx[ctxFsType] = fs
	}
	return &csipb.Volume{
		VolumeId:      volumeID,
		CapacityBytes: capacity,
		VolumeContext: ctx,
	}
}

// createNFSVolume provisions an NFS-mode volume by creating a ProtocolShare
// on the NovaNAS server. It is idempotent: an existing share with the same
// dataset is returned as-is.
//
// volumeID encoding stays "<pool>/<parent-after-pool>/<leaf>" so it remains
// addressable by the same ParseVolumeID path used for filesystem/block kinds.
func (s *ControllerService) createNFSVolume(ctx context.Context, params map[string]string, pool, parent, leaf, full string, capacity int64) (*csipb.CreateVolumeResponse, error) {
	// dataset within the pool, e.g. "csi-nfs/pvc-abc". The pool prefix is
	// stripped so ProtocolShareSpec.DatasetName matches the API contract.
	dsName := datasetSubpath(pool, full)
	if dsName == "" {
		return nil, errInvalid("derived empty dataset name from %q under pool %q", full, pool)
	}

	// Idempotency: GET first.
	existing, getErr := s.d.client.GetProtocolShare(ctx, leaf, pool, dsName)
	if getErr != nil && !s.d.client.IsNotFound(getErr) {
		return nil, errInternal("get protocol-share %s: %v", leaf, getErr)
	}
	if existing != nil {
		return &csipb.CreateVolumeResponse{Volume: s.buildNFSVolume(full, capacity, params, dsName)}, nil
	}

	// Build per-client export rules. StorageClass parameter "nfsClients"
	// (CSV) wins; otherwise the driver's --default-nfs-clients applies.
	clientCSV := firstNonEmpty(params[paramNFSClients], s.d.cfg.DefaultNFSClients)
	rules := buildNFSClientRules(clientCSV)
	// Empty rules are fine; the server may default to its global allowlist.

	spec := ProtocolShareSpec{
		Name:        leaf,
		Pool:        pool,
		DatasetName: dsName,
		Protocols:   []string{"nfs"},
		QuotaBytes:  capacity,
		NFSClients:  rules,
	}
	job, err := s.d.client.CreateProtocolShare(ctx, spec)
	if err != nil {
		return nil, errInternal("create protocol-share %s: %v", leaf, err)
	}
	if _, err := s.d.client.WaitJob(ctx, job.ID, JobPollInterval); err != nil {
		return nil, errInternal("wait create-protocol-share job: %v", err)
	}
	_ = parent // retained for symmetry with non-NFS path; not needed by the API.
	return &csipb.CreateVolumeResponse{Volume: s.buildNFSVolume(full, capacity, params, dsName)}, nil
}

// buildNFSVolume produces the CSI Volume returned for an NFS-mode volume.
// Pods receive nfsServer/nfsPath/mountOptions in VolumeContext so the Node
// service can mount.nfs4 without re-querying the API.
func (s *ControllerService) buildNFSVolume(volumeID string, capacity int64, params map[string]string, dsName string) *csipb.Volume {
	mountOpts := firstNonEmpty(params[paramMountOptions], DefaultNFSMountOptions)
	// The server-side export path is the dataset's filesystem mountpoint.
	// By convention, ProtocolShare.path = "/<pool>/<datasetName>" which
	// matches the dataset's default mountpoint.
	exportPath := "/" + extractPool(volumeID) + "/" + dsName
	return &csipb.Volume{
		VolumeId:      volumeID,
		CapacityBytes: capacity,
		VolumeContext: map[string]string{
			ctxVolumeKind:   volumeKindNFS,
			ctxNFSServer:    s.d.cfg.NFSServer,
			ctxNFSPath:      exportPath,
			ctxMountOptions: mountOpts,
		},
	}
}

// datasetSubpath strips the leading "<pool>/" prefix from a full ZFS name.
// Returns "" when full does not start with the pool.
func datasetSubpath(pool, full string) string {
	prefix := pool + "/"
	if !strings.HasPrefix(full, prefix) {
		return ""
	}
	return strings.TrimPrefix(full, prefix)
}

// extractPool returns the first segment of a "/"-separated ZFS path.
func extractPool(full string) string {
	if i := strings.Index(full, "/"); i > 0 {
		return full[:i]
	}
	return full
}

// buildNFSClientRules turns a comma-separated allowlist into per-spec rules
// with empty options. Options are owned by Agent C's host-side NFS layer
// (sec=krb5p is enforced in /etc/exports.d defaults), so the CSI driver
// passes only the spec.
func buildNFSClientRules(csv string) []NFSClientRule {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]NFSClientRule, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, NFSClientRule{Spec: p})
	}
	return out
}

// DeleteVolume destroys the volume. NotFound is success (idempotent).
//
// We don't carry kind=nfs in the VolumeID, so detection is done via
// GetProtocolShare: if a share exists for this leaf, full teardown goes
// through DeleteProtocolShare (samba+nfs+dataset). Otherwise we fall back
// to plain DestroyDataset for filesystem/block volumes.
func (s *ControllerService) DeleteVolume(ctx context.Context, req *csipb.DeleteVolumeRequest) (*csipb.DeleteVolumeResponse, error) {
	id, err := ParseVolumeID(req.GetVolumeId())
	if err != nil {
		return nil, errInvalid("VolumeId: %v", err)
	}
	dsName := datasetSubpath(id.Pool, id.Full)
	if dsName != "" {
		share, getErr := s.d.client.GetProtocolShare(ctx, id.Leaf, id.Pool, dsName)
		if getErr == nil && share != nil {
			job, derr := s.d.client.DeleteProtocolShare(ctx, id.Leaf, id.Pool, dsName)
			if derr != nil {
				if s.d.client.IsNotFound(derr) {
					return &csipb.DeleteVolumeResponse{}, nil
				}
				return nil, errInternal("delete protocol-share %s: %v", id.Leaf, derr)
			}
			if _, err := s.d.client.WaitJob(ctx, job.ID, JobPollInterval); err != nil {
				return nil, errInternal("wait delete-protocol-share job: %v", err)
			}
			return &csipb.DeleteVolumeResponse{}, nil
		}
		// getErr != nil and not NotFound: fall through to DestroyDataset
		// rather than failing — the share path is best-effort detection.
	}

	job, err := s.d.client.DestroyDataset(ctx, id.Full, false)
	if err != nil {
		if s.d.client.IsNotFound(err) {
			return &csipb.DeleteVolumeResponse{}, nil
		}
		return nil, errInternal("destroy dataset %s: %v", id.Full, err)
	}
	if _, err := s.d.client.WaitJob(ctx, job.ID, JobPollInterval); err != nil {
		return nil, errInternal("wait destroy-dataset job: %v", err)
	}
	return &csipb.DeleteVolumeResponse{}, nil
}

// ControllerExpandVolume grows quota (filesystem) or volsize (zvol).
func (s *ControllerService) ControllerExpandVolume(ctx context.Context, req *csipb.ControllerExpandVolumeRequest) (*csipb.ControllerExpandVolumeResponse, error) {
	id, err := ParseVolumeID(req.GetVolumeId())
	if err != nil {
		return nil, errInvalid("VolumeId: %v", err)
	}
	capacity := requestedCapacity(req.GetCapacityRange())
	ds, err := s.d.client.GetDataset(ctx, id.Full)
	if err != nil {
		if s.d.client.IsNotFound(err) {
			return nil, errNotFound("volume %s not found", id.Full)
		}
		return nil, errInternal("get dataset: %v", err)
	}

	var props map[string]string
	nodeExpansionRequired := false
	switch ds.Type {
	case "filesystem":
		capacity = roundUp(capacity, oneMiB)
		props = map[string]string{
			"quota":    strconv.FormatInt(capacity, 10),
			"refquota": strconv.FormatInt(capacity, 10),
		}
	case "volume":
		props = map[string]string{
			"volsize": strconv.FormatInt(capacity, 10),
		}
		// If the zvol carries a filesystem, kubelet must call NodeExpandVolume.
		// We can't know the fs from ZFS alone; assume required when a
		// VolumeCapability of type Mount was supplied.
		if c := req.GetVolumeCapability(); c != nil {
			if _, ok := c.GetAccessType().(*csipb.VolumeCapability_Mount); ok {
				nodeExpansionRequired = true
			}
		}
	default:
		return nil, errInternal("unknown dataset type %q", ds.Type)
	}

	job, err := s.d.client.SetDatasetProps(ctx, id.Full, props)
	if err != nil {
		return nil, errInternal("set props: %v", err)
	}
	if _, err := s.d.client.WaitJob(ctx, job.ID, JobPollInterval); err != nil {
		return nil, errInternal("wait set-props job: %v", err)
	}
	return &csipb.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: nodeExpansionRequired,
	}, nil
}

// ControllerGetVolume returns dataset state.
func (s *ControllerService) ControllerGetVolume(ctx context.Context, req *csipb.ControllerGetVolumeRequest) (*csipb.ControllerGetVolumeResponse, error) {
	id, err := ParseVolumeID(req.GetVolumeId())
	if err != nil {
		return nil, errInvalid("VolumeId: %v", err)
	}
	ds, err := s.d.client.GetDataset(ctx, id.Full)
	if err != nil {
		if s.d.client.IsNotFound(err) {
			return nil, errNotFound("volume %s", id.Full)
		}
		return nil, errInternal("get dataset: %v", err)
	}
	cap := datasetCapacity(ds)
	return &csipb.ControllerGetVolumeResponse{
		Volume: &csipb.Volume{
			VolumeId:      id.Full,
			CapacityBytes: cap,
			VolumeContext: map[string]string{ctxVolumeKind: ds.Type},
		},
	}, nil
}

// CreateSnapshot snapshots a source volume.
func (s *ControllerService) CreateSnapshot(ctx context.Context, req *csipb.CreateSnapshotRequest) (*csipb.CreateSnapshotResponse, error) {
	if req.GetName() == "" {
		return nil, errInvalid("Name is required")
	}
	src, err := ParseVolumeID(req.GetSourceVolumeId())
	if err != nil {
		return nil, errInvalid("SourceVolumeId: %v", err)
	}
	job, err := s.d.client.CreateSnapshot(ctx, src.Full, req.GetName(), false)
	if err != nil {
		return nil, errInternal("create snapshot: %v", err)
	}
	if _, err := s.d.client.WaitJob(ctx, job.ID, JobPollInterval); err != nil {
		return nil, errInternal("wait snapshot job: %v", err)
	}
	snapID := EncodeSnapshotID(src.Full, req.GetName())
	return &csipb.CreateSnapshotResponse{
		Snapshot: &csipb.Snapshot{
			SnapshotId:     snapID,
			SourceVolumeId: src.Full,
			ReadyToUse:     true,
		},
	}, nil
}

// DeleteSnapshot is idempotent.
func (s *ControllerService) DeleteSnapshot(ctx context.Context, req *csipb.DeleteSnapshotRequest) (*csipb.DeleteSnapshotResponse, error) {
	if req.GetSnapshotId() == "" {
		return nil, errInvalid("SnapshotId is required")
	}
	if _, err := ParseSnapshotID(req.GetSnapshotId()); err != nil {
		return nil, errInvalid("SnapshotId: %v", err)
	}
	job, err := s.d.client.DestroySnapshot(ctx, req.GetSnapshotId())
	if err != nil {
		if s.d.client.IsNotFound(err) {
			return &csipb.DeleteSnapshotResponse{}, nil
		}
		return nil, errInternal("destroy snapshot: %v", err)
	}
	if _, err := s.d.client.WaitJob(ctx, job.ID, JobPollInterval); err != nil {
		return nil, errInternal("wait destroy-snapshot job: %v", err)
	}
	return &csipb.DeleteSnapshotResponse{}, nil
}

// ValidateVolumeCapabilities confirms supported access modes.
func (s *ControllerService) ValidateVolumeCapabilities(ctx context.Context, req *csipb.ValidateVolumeCapabilitiesRequest) (*csipb.ValidateVolumeCapabilitiesResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, errInvalid("VolumeId is required")
	}
	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, errInvalid("VolumeCapabilities is required")
	}
	kind, err := classify(req.GetVolumeCapabilities(), req.GetParameters())
	if err != nil {
		return &csipb.ValidateVolumeCapabilitiesResponse{Message: err.Error()}, nil
	}
	for _, c := range req.GetVolumeCapabilities() {
		mode := c.GetAccessMode().GetMode()
		if !accessModeSupported(kind, mode) {
			return &csipb.ValidateVolumeCapabilitiesResponse{
				Message: fmt.Sprintf("access mode %s not supported", mode),
			}, nil
		}
	}
	return &csipb.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csipb.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}, nil
}

// accessModeSupported: zvols are RWO only; filesystem datasets accept RWO and
// RWX (single-node only — kubelet enforces node affinity); NFS-mode volumes
// support every mode including true cross-node RWX (the whole point).
func accessModeSupported(kind volumeKind, mode csipb.VolumeCapability_AccessMode_Mode) bool {
	switch kind {
	case kindBlock:
		return mode == csipb.VolumeCapability_AccessMode_SINGLE_NODE_WRITER ||
			mode == csipb.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER
	case kindFilesystem, kindNFS:
		switch mode {
		case csipb.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csipb.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
			csipb.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
			csipb.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
			csipb.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER:
			return true
		}
	}
	return false
}

// ListVolumes / ListSnapshots are not implemented in v1.
func (s *ControllerService) ListVolumes(ctx context.Context, req *csipb.ListVolumesRequest) (*csipb.ListVolumesResponse, error) {
	return nil, errUnimplemented("ListVolumes")
}
func (s *ControllerService) ListSnapshots(ctx context.Context, req *csipb.ListSnapshotsRequest) (*csipb.ListSnapshotsResponse, error) {
	return nil, errUnimplemented("ListSnapshots")
}

// helpers

func zfsType(k volumeKind) string {
	switch k {
	case kindBlock:
		return "volume"
	case kindNFS:
		// NFS-mode volumes are backed by filesystem datasets; the node
		// service distinguishes them via VolumeContext.volumeKind = "nfs".
		return "filesystem"
	default:
		return "filesystem"
	}
}

func requestedCapacity(r *csipb.CapacityRange) int64 {
	if r == nil {
		return defaultCapacityBytes
	}
	if r.RequiredBytes > 0 {
		return r.RequiredBytes
	}
	if r.LimitBytes > 0 {
		return r.LimitBytes
	}
	return defaultCapacityBytes
}

func roundUp(n, m int64) int64 {
	if m <= 0 {
		return n
	}
	if n%m == 0 {
		return n
	}
	return ((n / m) + 1) * m
}

func sizeCompatible(ds *Dataset, kind volumeKind, want int64) bool {
	switch kind {
	case kindBlock:
		// Volsize must match (allow empty if API didn't populate it).
		return ds.Volsize == 0 || ds.Volsize == want
	case kindFilesystem:
		// Filesystem datasets: quota must match if set.
		return ds.Quota == 0 || ds.Quota == want
	}
	return false
}

func datasetCapacity(ds *Dataset) int64 {
	if ds.Volsize > 0 {
		return ds.Volsize
	}
	if ds.Quota > 0 {
		return ds.Quota
	}
	return ds.UsedBytes + ds.AvailableBytes
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
