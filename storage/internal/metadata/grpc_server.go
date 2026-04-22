package metadata

import (
	"context"
	"errors"

	pb "github.com/azrtydxb/novanas/storage/api/proto/metadata"
	"github.com/azrtydxb/novanas/storage/internal/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GRPCServer implements the MetadataService using typed protobuf RPCs.
type GRPCServer struct {
	pb.UnimplementedMetadataServiceServer
	store     *RaftStore
	sbs       *SuperblockRegistry
	sbsMinDks int
}

// NewGRPCServer creates a new GRPCServer backed by the given RaftStore.
// The server exposes an in-memory SuperblockRegistry for ReportSuperblocks;
// callers that need to inspect bootstrap state can fetch it via
// SuperblockRegistry(). MinMetadataDisks defaults to 1.
func NewGRPCServer(store *RaftStore) *GRPCServer {
	return &GRPCServer{
		store:     store,
		sbs:       NewSuperblockRegistry(),
		sbsMinDks: 1,
	}
}

// WithSuperblockRegistry overrides the server's in-memory registry. Use
// this when bootstrap code needs to share the same registry across the
// gRPC server and the metadata bootstrap path.
func (s *GRPCServer) WithSuperblockRegistry(r *SuperblockRegistry, minMetadataDisks int) *GRPCServer {
	if r != nil {
		s.sbs = r
	}
	if minMetadataDisks > 0 {
		s.sbsMinDks = minMetadataDisks
	}
	return s
}

// SuperblockRegistry returns the server's in-memory registry so callers
// (e.g. the bootstrap path) can reuse it as a SuperblockSource.
func (s *GRPCServer) SuperblockRegistry() *SuperblockRegistry { return s.sbs }

// Register adds the MetadataService to a gRPC server.
func (s *GRPCServer) Register(srv *grpc.Server) {
	pb.RegisterMetadataServiceServer(srv, s)
}

// storeErr maps store-level sentinel errors to gRPC status codes.
func storeErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrKeyNotFound) {
		return status.Error(codes.NotFound, err.Error())
	}
	if errors.Is(err, ErrBucketNotFound) {
		return status.Error(codes.NotFound, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}

// ---- Volume operations ----

func (s *GRPCServer) PutVolumeMeta(ctx context.Context, req *pb.PutVolumeMetaRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("PutVolumeMeta").Inc()
	if req.Meta == nil {
		return nil, status.Error(codes.InvalidArgument, "meta is required")
	}
	meta := VolumeMetaFromProto(req.Meta)
	if err := s.store.PutVolumeMeta(ctx, meta); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) GetVolumeMeta(ctx context.Context, req *pb.GetVolumeMetaRequest) (*pb.GetVolumeMetaResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetVolumeMeta").Inc()
	meta, err := s.store.GetVolumeMeta(ctx, req.VolumeId)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.GetVolumeMetaResponse{Meta: VolumeMetaToProto(meta)}, nil
}

func (s *GRPCServer) DeleteVolumeMeta(ctx context.Context, req *pb.DeleteVolumeMetaRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("DeleteVolumeMeta").Inc()
	if err := s.store.DeleteVolumeMeta(ctx, req.VolumeId); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) ListVolumesMeta(ctx context.Context, _ *emptypb.Empty) (*pb.ListVolumesMetaResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("ListVolumesMeta").Inc()
	metas, err := s.store.ListVolumesMeta(ctx)
	if err != nil {
		return nil, storeErr(err)
	}
	resp := &pb.ListVolumesMetaResponse{Metas: make([]*pb.VolumeMeta, len(metas))}
	for i, m := range metas {
		resp.Metas[i] = VolumeMetaToProto(m)
	}
	return resp, nil
}

// ---- Placement operations ----

func (s *GRPCServer) PutPlacementMap(ctx context.Context, req *pb.PutPlacementMapRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("PutPlacementMap").Inc()
	if req.PlacementMap == nil {
		return nil, status.Error(codes.InvalidArgument, "placement_map is required")
	}
	pm := PlacementMapFromProto(req.PlacementMap)
	if err := s.store.PutPlacementMap(ctx, pm); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) GetPlacementMap(ctx context.Context, req *pb.GetPlacementMapRequest) (*pb.GetPlacementMapResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetPlacementMap").Inc()
	pm, err := s.store.GetPlacementMap(ctx, req.ChunkId)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.GetPlacementMapResponse{PlacementMap: PlacementMapToProto(pm)}, nil
}

func (s *GRPCServer) DeletePlacementMap(ctx context.Context, req *pb.DeletePlacementMapRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("DeletePlacementMap").Inc()
	if err := s.store.DeletePlacementMap(ctx, req.ChunkId); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) ListPlacementMaps(ctx context.Context, _ *emptypb.Empty) (*pb.ListPlacementMapsResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("ListPlacementMaps").Inc()
	pms, err := s.store.ListPlacementMaps(ctx)
	if err != nil {
		return nil, storeErr(err)
	}
	resp := &pb.ListPlacementMapsResponse{PlacementMaps: make([]*pb.PlacementMap, len(pms))}
	for i, pm := range pms {
		resp.PlacementMaps[i] = PlacementMapToProto(pm)
	}
	return resp, nil
}

// ---- Object operations ----

func (s *GRPCServer) PutObjectMeta(ctx context.Context, req *pb.PutObjectMetaRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("PutObjectMeta").Inc()
	if req.Meta == nil {
		return nil, status.Error(codes.InvalidArgument, "meta is required")
	}
	meta := ObjectMetaFromProto(req.Meta)
	if err := s.store.PutObjectMeta(ctx, meta); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) GetObjectMeta(ctx context.Context, req *pb.GetObjectMetaRequest) (*pb.GetObjectMetaResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetObjectMeta").Inc()
	meta, err := s.store.GetObjectMeta(ctx, req.Bucket, req.Key)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.GetObjectMetaResponse{Meta: ObjectMetaToProto(meta)}, nil
}

func (s *GRPCServer) DeleteObjectMeta(ctx context.Context, req *pb.DeleteObjectMetaRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("DeleteObjectMeta").Inc()
	if err := s.store.DeleteObjectMeta(ctx, req.Bucket, req.Key); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) ListObjectMetas(ctx context.Context, req *pb.ListObjectMetasRequest) (*pb.ListObjectMetasResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("ListObjectMetas").Inc()
	metas, err := s.store.ListObjectMetas(ctx, req.Bucket, req.Prefix)
	if err != nil {
		return nil, storeErr(err)
	}
	resp := &pb.ListObjectMetasResponse{Metas: make([]*pb.ObjectMeta, len(metas))}
	for i, m := range metas {
		resp.Metas[i] = ObjectMetaToProto(m)
	}
	return resp, nil
}

// ---- Bucket operations ----

func (s *GRPCServer) PutBucketMeta(ctx context.Context, req *pb.PutBucketMetaRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("PutBucketMeta").Inc()
	if req.Meta == nil {
		return nil, status.Error(codes.InvalidArgument, "meta is required")
	}
	meta := BucketMetaFromProto(req.Meta)
	if err := s.store.PutBucketMeta(ctx, meta); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) GetBucketMeta(ctx context.Context, req *pb.GetBucketMetaRequest) (*pb.GetBucketMetaResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetBucketMeta").Inc()
	meta, err := s.store.GetBucketMeta(ctx, req.Name)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.GetBucketMetaResponse{Meta: BucketMetaToProto(meta)}, nil
}

func (s *GRPCServer) DeleteBucketMeta(ctx context.Context, req *pb.DeleteBucketMetaRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("DeleteBucketMeta").Inc()
	if err := s.store.DeleteBucketMeta(ctx, req.Name); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) ListBucketMetas(ctx context.Context, _ *emptypb.Empty) (*pb.ListBucketMetasResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("ListBucketMetas").Inc()
	metas, err := s.store.ListBucketMetas(ctx)
	if err != nil {
		return nil, storeErr(err)
	}
	resp := &pb.ListBucketMetasResponse{Metas: make([]*pb.BucketMeta, len(metas))}
	for i, m := range metas {
		resp.Metas[i] = BucketMetaToProto(m)
	}
	return resp, nil
}

// ---- Multipart operations ----

func (s *GRPCServer) PutMultipartUpload(ctx context.Context, req *pb.PutMultipartUploadRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("PutMultipartUpload").Inc()
	if req.Upload == nil {
		return nil, status.Error(codes.InvalidArgument, "upload is required")
	}
	mu := MultipartUploadFromProto(req.Upload)
	if err := s.store.PutMultipartUpload(ctx, mu); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) GetMultipartUpload(ctx context.Context, req *pb.GetMultipartUploadRequest) (*pb.GetMultipartUploadResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetMultipartUpload").Inc()
	mu, err := s.store.GetMultipartUpload(ctx, req.UploadId)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.GetMultipartUploadResponse{Upload: MultipartUploadToProto(mu)}, nil
}

func (s *GRPCServer) DeleteMultipartUpload(ctx context.Context, req *pb.DeleteMultipartUploadRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("DeleteMultipartUpload").Inc()
	if err := s.store.DeleteMultipartUpload(ctx, req.UploadId); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

// ---- Snapshot operations ----

func (s *GRPCServer) PutSnapshot(ctx context.Context, req *pb.PutSnapshotRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("PutSnapshot").Inc()
	if req.Meta == nil {
		return nil, status.Error(codes.InvalidArgument, "meta is required")
	}
	meta := SnapshotMetaFromProto(req.Meta)
	if err := s.store.PutSnapshot(ctx, meta); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) GetSnapshot(ctx context.Context, req *pb.GetSnapshotRequest) (*pb.GetSnapshotResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetSnapshot").Inc()
	meta, err := s.store.GetSnapshot(ctx, req.SnapshotId)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.GetSnapshotResponse{Meta: SnapshotMetaToProto(meta)}, nil
}

func (s *GRPCServer) DeleteSnapshot(ctx context.Context, req *pb.DeleteSnapshotRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("DeleteSnapshot").Inc()
	if err := s.store.DeleteSnapshot(ctx, req.SnapshotId); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) ListSnapshots(ctx context.Context, _ *emptypb.Empty) (*pb.ListSnapshotsResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("ListSnapshots").Inc()
	metas, err := s.store.ListSnapshots(ctx)
	if err != nil {
		return nil, storeErr(err)
	}
	resp := &pb.ListSnapshotsResponse{Metas: make([]*pb.SnapshotMeta, len(metas))}
	for i, m := range metas {
		resp.Metas[i] = SnapshotMetaToProto(m)
	}
	return resp, nil
}

// ---- Inode operations ----

func (s *GRPCServer) CreateInode(ctx context.Context, req *pb.CreateInodeRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("CreateInode").Inc()
	if req.Meta == nil {
		return nil, status.Error(codes.InvalidArgument, "meta is required")
	}
	meta := InodeMetaFromProto(req.Meta)
	if err := s.store.CreateInode(ctx, meta); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) GetInode(ctx context.Context, req *pb.GetInodeRequest) (*pb.GetInodeResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetInode").Inc()
	meta, err := s.store.GetInode(ctx, req.Ino)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.GetInodeResponse{Meta: InodeMetaToProto(meta)}, nil
}

func (s *GRPCServer) UpdateInode(ctx context.Context, req *pb.UpdateInodeRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("UpdateInode").Inc()
	if req.Meta == nil {
		return nil, status.Error(codes.InvalidArgument, "meta is required")
	}
	meta := InodeMetaFromProto(req.Meta)
	if err := s.store.UpdateInode(ctx, meta); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) DeleteInode(ctx context.Context, req *pb.DeleteInodeRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("DeleteInode").Inc()
	if err := s.store.DeleteInode(ctx, req.Ino); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) AllocateIno(ctx context.Context, _ *emptypb.Empty) (*pb.AllocateInoResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("AllocateIno").Inc()
	ino, err := s.store.AllocateIno(ctx)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.AllocateInoResponse{Ino: ino}, nil
}

// ---- Directory entry operations ----

func (s *GRPCServer) CreateDirEntry(ctx context.Context, req *pb.CreateDirEntryRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("CreateDirEntry").Inc()
	if req.Entry == nil {
		return nil, status.Error(codes.InvalidArgument, "entry is required")
	}
	entry := DirEntryFromProto(req.Entry)
	if err := s.store.CreateDirEntry(ctx, req.ParentIno, entry); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) DeleteDirEntry(ctx context.Context, req *pb.DeleteDirEntryRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("DeleteDirEntry").Inc()
	if err := s.store.DeleteDirEntry(ctx, req.ParentIno, req.Name); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) LookupDirEntry(ctx context.Context, req *pb.LookupDirEntryRequest) (*pb.LookupDirEntryResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("LookupDirEntry").Inc()
	entry, err := s.store.LookupDirEntry(ctx, req.ParentIno, req.Name)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.LookupDirEntryResponse{Entry: DirEntryToProto(entry)}, nil
}

func (s *GRPCServer) ListDirectory(ctx context.Context, req *pb.ListDirectoryRequest) (*pb.ListDirectoryResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("ListDirectory").Inc()
	entries, err := s.store.ListDirectory(ctx, req.ParentIno)
	if err != nil {
		return nil, storeErr(err)
	}
	resp := &pb.ListDirectoryResponse{Entries: make([]*pb.DirEntry, len(entries))}
	for i, e := range entries {
		resp.Entries[i] = DirEntryToProto(e)
	}
	return resp, nil
}

// ---- Node operations ----

func (s *GRPCServer) PutNodeMeta(ctx context.Context, req *pb.PutNodeMetaRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("PutNodeMeta").Inc()
	if req.Meta == nil {
		return nil, status.Error(codes.InvalidArgument, "meta is required")
	}
	meta := NodeMetaFromProto(req.Meta)
	if err := s.store.PutNodeMeta(ctx, meta); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) GetNodeMeta(ctx context.Context, req *pb.GetNodeMetaRequest) (*pb.GetNodeMetaResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetNodeMeta").Inc()
	meta, err := s.store.GetNodeMeta(ctx, req.NodeId)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.GetNodeMetaResponse{Meta: NodeMetaToProto(meta)}, nil
}

func (s *GRPCServer) DeleteNodeMeta(ctx context.Context, req *pb.DeleteNodeMetaRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("DeleteNodeMeta").Inc()
	if err := s.store.DeleteNodeMeta(ctx, req.NodeId); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) ListNodeMetas(ctx context.Context, _ *emptypb.Empty) (*pb.ListNodeMetasResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("ListNodeMetas").Inc()
	metas, err := s.store.ListNodeMetas(ctx)
	if err != nil {
		return nil, storeErr(err)
	}
	resp := &pb.ListNodeMetasResponse{Metas: make([]*pb.NodeMeta, len(metas))}
	for i, m := range metas {
		resp.Metas[i] = NodeMetaToProto(m)
	}
	return resp, nil
}

// ---- Lock operations ----

func (s *GRPCServer) AcquireLock(ctx context.Context, req *pb.AcquireLockRequest) (*pb.AcquireLockResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("AcquireLock").Inc()
	args := AcquireLockArgsFromProto(req)
	result, err := s.store.AcquireLock(ctx, args)
	if err != nil {
		return nil, storeErr(err)
	}
	return AcquireLockResultToProto(result), nil
}

func (s *GRPCServer) RenewLock(ctx context.Context, req *pb.RenewLockRequest) (*pb.RenewLockResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("RenewLock").Inc()
	args := RenewLockArgsFromProto(req)
	lease, err := s.store.RenewLock(ctx, args)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.RenewLockResponse{Lease: LockLeaseToProto(lease)}, nil
}

func (s *GRPCServer) ReleaseLock(ctx context.Context, req *pb.ReleaseLockRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("ReleaseLock").Inc()
	args := ReleaseLockArgsFromProto(req)
	if err := s.store.ReleaseLock(ctx, args); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) TestLock(ctx context.Context, req *pb.TestLockRequest) (*pb.TestLockResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("TestLock").Inc()
	args := TestLockArgsFromProto(req)
	lease, err := s.store.TestLock(ctx, args)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.TestLockResponse{ConflictingLock: LockLeaseToProto(lease)}, nil
}

func (s *GRPCServer) GetLock(ctx context.Context, req *pb.GetLockRequest) (*pb.GetLockResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetLock").Inc()
	lease, err := s.store.GetLock(ctx, req.LeaseId)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.GetLockResponse{Lease: LockLeaseToProto(lease)}, nil
}

func (s *GRPCServer) ListLocks(ctx context.Context, req *pb.ListLocksRequest) (*pb.ListLocksResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("ListLocks").Inc()
	locks, err := s.store.ListLocks(ctx, req.VolumeId)
	if err != nil {
		return nil, storeErr(err)
	}
	resp := &pb.ListLocksResponse{Locks: make([]*pb.LockLease, len(locks))}
	for i, l := range locks {
		resp.Locks[i] = LockLeaseToProto(l)
	}
	return resp, nil
}

func (s *GRPCServer) CleanupExpiredLocks(ctx context.Context, _ *emptypb.Empty) (*pb.CleanupExpiredLocksResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("CleanupExpiredLocks").Inc()
	count, err := s.store.CleanupExpiredLocks(ctx)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.CleanupExpiredLocksResponse{Cleaned: int32(count)}, nil
}

// ---- Volume ownership operations ----

func (s *GRPCServer) SetVolumeOwner(_ context.Context, req *pb.SetVolumeOwnerRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("SetVolumeOwner").Inc()
	if req.Ownership == nil {
		return nil, status.Error(codes.InvalidArgument, "ownership is required")
	}
	ownership := VolumeOwnershipFromProto(req.Ownership)
	if err := s.store.SetVolumeOwner(ownership); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) GetVolumeOwner(_ context.Context, req *pb.GetVolumeOwnerRequest) (*pb.GetVolumeOwnerResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetVolumeOwner").Inc()
	ownership, err := s.store.GetVolumeOwner(req.VolumeId)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.GetVolumeOwnerResponse{Ownership: VolumeOwnershipToProto(ownership)}, nil
}

func (s *GRPCServer) RequestOwnership(_ context.Context, req *pb.RequestOwnershipRequest) (*pb.RequestOwnershipResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("RequestOwnership").Inc()
	granted, generation, err := s.store.RequestOwnership(req.VolumeId, req.RequesterAddr)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.RequestOwnershipResponse{Granted: granted, Generation: generation}, nil
}

// ---- Shard placement operations (erasure coding) ----

func (s *GRPCServer) PutShardPlacement(ctx context.Context, req *pb.PutShardPlacementRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("PutShardPlacement").Inc()
	if req.Placement == nil {
		return nil, status.Error(codes.InvalidArgument, "placement is required")
	}
	sp := ShardPlacementFromProto(req.Placement)
	if err := s.store.PutShardPlacement(ctx, sp); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) GetShardPlacements(ctx context.Context, req *pb.GetShardPlacementsRequest) (*pb.GetShardPlacementsResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetShardPlacements").Inc()
	placements, err := s.store.GetShardPlacements(ctx, req.ChunkId)
	if err != nil {
		return nil, storeErr(err)
	}
	resp := &pb.GetShardPlacementsResponse{Placements: make([]*pb.ShardPlacementMsg, len(placements))}
	for i, sp := range placements {
		resp.Placements[i] = ShardPlacementToProto(sp)
	}
	return resp, nil
}

func (s *GRPCServer) DeleteShardPlacement(ctx context.Context, req *pb.DeleteShardPlacementRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("DeleteShardPlacement").Inc()
	if err := s.store.DeleteShardPlacement(ctx, req.ChunkId, int(req.ShardIndex)); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

// ---- Heal task operations ----

func (s *GRPCServer) PutHealTask(ctx context.Context, req *pb.PutHealTaskRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("PutHealTask").Inc()
	if req.Task == nil {
		return nil, status.Error(codes.InvalidArgument, "task is required")
	}
	task := HealTaskFromProto(req.Task)
	if err := s.store.PutHealTask(ctx, task); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) GetHealTask(ctx context.Context, req *pb.GetHealTaskRequest) (*pb.GetHealTaskResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetHealTask").Inc()
	task, err := s.store.GetHealTask(ctx, req.Id)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.GetHealTaskResponse{Task: HealTaskToProto(task)}, nil
}

func (s *GRPCServer) ListPendingHealTasks(ctx context.Context, _ *emptypb.Empty) (*pb.ListPendingHealTasksResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("ListPendingHealTasks").Inc()
	tasks, err := s.store.ListPendingHealTasks(ctx)
	if err != nil {
		return nil, storeErr(err)
	}
	resp := &pb.ListPendingHealTasksResponse{Tasks: make([]*pb.HealTaskMsg, len(tasks))}
	for i, t := range tasks {
		resp.Tasks[i] = HealTaskToProto(t)
	}
	return resp, nil
}

func (s *GRPCServer) ListHealTasksByVolume(ctx context.Context, req *pb.ListHealTasksByVolumeRequest) (*pb.ListHealTasksByVolumeResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("ListHealTasksByVolume").Inc()
	tasks, err := s.store.ListHealTasksByVolume(ctx, req.VolumeId)
	if err != nil {
		return nil, storeErr(err)
	}
	resp := &pb.ListHealTasksByVolumeResponse{Tasks: make([]*pb.HealTaskMsg, len(tasks))}
	for i, t := range tasks {
		resp.Tasks[i] = HealTaskToProto(t)
	}
	return resp, nil
}

func (s *GRPCServer) DeleteHealTask(ctx context.Context, req *pb.DeleteHealTaskRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("DeleteHealTask").Inc()
	if err := s.store.DeleteHealTask(ctx, req.Id); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

// ---- Quota operations ----

func (s *GRPCServer) SetQuota(ctx context.Context, req *pb.SetQuotaRequest) (*emptypb.Empty, error) {
	metrics.MetadataOpsTotal.WithLabelValues("SetQuota").Inc()
	if req.Spec == nil {
		return nil, status.Error(codes.InvalidArgument, "spec is required")
	}
	scope, spec := QuotaSpecFromProto(req.Spec)
	qs := NewQuotaStore(s.store)
	if err := qs.SetQuota(ctx, scope, spec); err != nil {
		return nil, storeErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *GRPCServer) GetUsage(ctx context.Context, req *pb.GetUsageRequest) (*pb.GetUsageResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetUsage").Inc()
	scope := QuotaScope{Kind: req.Kind, Name: req.Name}
	qs := NewQuotaStore(s.store)
	usage, err := qs.GetUsage(ctx, scope)
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.GetUsageResponse{Usage: QuotaUsageToProto(scope, usage)}, nil
}

// ---- Cluster management operations ----

// JoinCluster is retained for gRPC API compatibility but is now a no-op:
// NovaNas is single-node by design (docs/14 S12), so there is no cluster
// to join. The request is validated and acknowledged so that existing
// clients (operators, CSI, s3gw) continue to work without modification.
func (s *GRPCServer) JoinCluster(_ context.Context, req *pb.JoinClusterRequest) (*pb.JoinClusterResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("JoinCluster").Inc()
	if req.NodeId == "" || req.RaftAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id and raft_address required")
	}
	return &pb.JoinClusterResponse{Success: true}, nil
}

// ---- Per-chunk crypto metadata ----

func (s *GRPCServer) SetChunkCrypto(ctx context.Context, req *pb.SetChunkCryptoRequest) (*pb.SetChunkCryptoResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("SetChunkCrypto").Inc()
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id is required")
	}
	c := req.GetCrypto()
	if c == nil || c.GetChunkId() == "" {
		return nil, status.Error(codes.InvalidArgument, "crypto.chunk_id is required")
	}
	entry := ChunkCryptoEntry{
		PlaintextHash: c.GetPlaintextHash(),
		AuthTag:       c.GetAuthTag(),
		DKVersion:     c.GetDkVersion(),
	}
	if err := s.store.SetChunkCrypto(ctx, req.GetVolumeId(), c.GetChunkId(), entry); err != nil {
		return nil, storeErr(err)
	}
	return &pb.SetChunkCryptoResponse{}, nil
}

func (s *GRPCServer) GetChunkCrypto(ctx context.Context, req *pb.GetChunkCryptoRequest) (*pb.GetChunkCryptoResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("GetChunkCrypto").Inc()
	if req.GetVolumeId() == "" || req.GetChunkId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id and chunk_id are required")
	}
	entry, err := s.store.GetChunkCrypto(ctx, req.GetVolumeId(), req.GetChunkId())
	if err != nil {
		return nil, storeErr(err)
	}
	return &pb.GetChunkCryptoResponse{Crypto: &pb.ChunkCrypto{
		ChunkId:       req.GetChunkId(),
		PlaintextHash: entry.PlaintextHash,
		AuthTag:       entry.AuthTag,
		DkVersion:     entry.DKVersion,
	}}, nil
}

func (s *GRPCServer) DeleteChunkCrypto(ctx context.Context, req *pb.DeleteChunkCryptoRequest) (*pb.DeleteChunkCryptoResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("DeleteChunkCrypto").Inc()
	if req.GetVolumeId() == "" || req.GetChunkId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id and chunk_id are required")
	}
	if err := s.store.DeleteChunkCrypto(ctx, req.GetVolumeId(), req.GetChunkId()); err != nil {
		return nil, storeErr(err)
	}
	return &pb.DeleteChunkCryptoResponse{}, nil
}

// ---- Bootstrap ----

// ReportSuperblocks accepts a batch of per-disk superblock descriptors
// from an agent at startup and records them in the server's in-memory
// SuperblockRegistry. The metadata bootstrap code reads from the same
// registry (SuperblockSource), resolving the chicken-and-egg problem
// documented in bootstrap.go.
func (s *GRPCServer) ReportSuperblocks(_ context.Context, req *pb.ReportSuperblocksRequest) (*pb.ReportSuperblocksResponse, error) {
	metrics.MetadataOpsTotal.WithLabelValues("ReportSuperblocks").Inc()
	if req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}
	if s.sbs == nil {
		return nil, status.Error(codes.FailedPrecondition, "superblock registry not configured")
	}
	reports := superblockProtosToScanResults(req.GetSuperblocks())
	accepted, total := s.sbs.Ingest(req.GetNodeId(), reports)
	min := s.sbsMinDks
	if min <= 0 {
		min = 1
	}
	return &pb.ReportSuperblocksResponse{
		AcceptedCount:     uint32(accepted),
		QuorumReached:     total >= min,
		MetadataDisksSeen: uint32(total),
	}, nil
}
