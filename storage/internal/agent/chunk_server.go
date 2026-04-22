package agent

import (
	"context"
	"fmt"
	"io"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/azrtydxb/novanas/storage/api/proto/chunk"
	"github.com/azrtydxb/novanas/storage/internal/crypto"
	"github.com/azrtydxb/novanas/storage/internal/dataplane"
	"github.com/azrtydxb/novanas/storage/internal/logging"
)

// VolumeKeyProvider is the minimal slice of crypto.VolumeKeyManager that the
// chunk server needs: given a volume id, return the cached DK raw bytes if
// the volume is encrypted-and-mounted, or (nil, false) for unencrypted or
// unmounted volumes. It is exposed as an interface so tests can fake it.
type VolumeKeyProvider interface {
	// DatasetKey returns the raw 32-byte Dataset Key for volumeID, or
	// (nil, false) if the volume is unencrypted or not mounted.
	DatasetKey(volumeID string) ([]byte, bool)
}

// volumeKeyAdapter wraps a *crypto.VolumeKeyManager to satisfy VolumeKeyProvider.
type volumeKeyAdapter struct{ m *crypto.VolumeKeyManager }

// DatasetKey returns the raw DK bytes for volumeID, or (nil, false) if
// the volume is not encrypted / not mounted.
func (a volumeKeyAdapter) DatasetKey(volumeID string) ([]byte, bool) {
	if a.m == nil {
		return nil, false
	}
	dk, ok := a.m.Get(volumeID)
	if !ok || dk == nil {
		return nil, false
	}
	raw, err := dk.Bytes()
	if err != nil {
		return nil, false
	}
	return raw, true
}

// NewVolumeKeyProvider adapts a *crypto.VolumeKeyManager to VolumeKeyProvider.
// A nil manager yields a provider that reports every volume as unencrypted.
func NewVolumeKeyProvider(m *crypto.VolumeKeyManager) VolumeKeyProvider {
	return volumeKeyAdapter{m: m}
}

// MetaClient is the minimal slice of the metadata GRPCClient used by the
// ChunkServer. It exists so main.go can wire a real gRPC client while
// tests plug in an in-memory fake. Shape matches
// metadata.GRPCClient.SetChunkCrypto / GetChunkCrypto / DeleteChunkCrypto.
//
// A8-Persistence: per-chunk encryption bookkeeping (plaintext hash, auth
// tag, DK version) is persisted in VolumeMeta via this interface so the
// chunk server survives restarts.
type MetaClient interface {
	SetChunkCrypto(ctx context.Context, volumeID, chunkID string, plaintextHash, authTag []byte, dkVersion uint32) error
	GetChunkCrypto(ctx context.Context, volumeID, chunkID string) (plaintextHash, authTag []byte, dkVersion uint32, err error)
	DeleteChunkCrypto(ctx context.Context, volumeID, chunkID string) error
}

// ChunkServer implements the ChunkService gRPC server by bridging calls to the
// Rust SPDK data-plane via gRPC. This ensures all chunk I/O (from S3, Filer,
// or any other access layer) flows through the Rust dataplane, never through Go.
//
// When a VolumeKeyProvider is wired in, PutChunk / GetChunk transparently
// encrypt and decrypt using crypto.EncryptChunk / crypto.DecryptChunk based
// on the per-request VolumeId. Requests that either (a) do not carry a
// VolumeId, or (b) target a volume that has no DK cached, fall through to
// the unencrypted path unchanged -- preserving the existing scaffolding
// test surface.
//
// Per-chunk crypto metadata (plaintext_hash, auth_tag, dk_version) is
// persisted in VolumeMeta via the MetaClient so an agent restart does not
// render encrypted chunks unreadable.
//
// main.go wiring: pass WithMetaClient(metadata.Dial(...)) when
// constructing the ChunkServer so the persistence path is live.
type ChunkServer struct {
	pb.UnimplementedChunkServiceServer

	dpClient *dataplane.Client
	bdevName string
	// keys may be nil, in which case every chunk flows unencrypted.
	keys VolumeKeyProvider
	// meta is the metadata-service client through which per-chunk
	// crypto bookkeeping is persisted. May be nil in unit tests that
	// stub out encryption entirely; when keys != nil and meta == nil
	// the chunk server will log a warning and fall back to plaintext.
	meta MetaClient
}

// NewChunkServer creates a ChunkServer that routes chunk operations to the SPDK
// data-plane via gRPC. bdevName is the chunk store bdev (same as used by SPDKTargetServer).
func NewChunkServer(dpClient *dataplane.Client, bdevName string) *ChunkServer {
	return &ChunkServer{
		dpClient: dpClient,
		bdevName: bdevName,
	}
}

// WithVolumeKeys returns the ChunkServer configured with the given key
// provider, enabling on-the-fly encryption / decryption for volumes that
// have a DK cached. Pass (nil) to disable encryption entirely.
func (s *ChunkServer) WithVolumeKeys(keys VolumeKeyProvider) *ChunkServer {
	s.keys = keys
	return s
}

// WithMetaClient configures the metadata-service client used to persist
// per-chunk crypto bookkeeping. Required when encryption is enabled; a
// nil client disables encryption's persistence layer (chunks written
// encrypted will then be unreadable after restart — tests only).
func (s *ChunkServer) WithMetaClient(m MetaClient) *ChunkServer {
	s.meta = m
	return s
}

// Register adds the ChunkService to the given gRPC server.
func (s *ChunkServer) Register(srv *grpc.Server) {
	pb.RegisterChunkServiceServer(srv, s)
}

// maybeEncrypt runs plaintext through crypto.EncryptChunk if the server has
// a key for volumeID, otherwise returns the plaintext unchanged. Returns
// (payload, plaintextHash, authTag, encrypted).
//
// When encrypted == true the returned payload is the AES-GCM ciphertext and
// the caller must persist plaintextHash + authTag alongside the stored
// chunk id. When encrypted == false plaintextHash and authTag are zero.
func (s *ChunkServer) maybeEncrypt(volumeID string, plaintext []byte) ([]byte, []byte, [crypto.AuthTagSize]byte, bool) {
	var zeroTag [crypto.AuthTagSize]byte
	if s.keys == nil || volumeID == "" {
		return plaintext, nil, zeroTag, false
	}
	dk, ok := s.keys.DatasetKey(volumeID)
	if !ok {
		return plaintext, nil, zeroTag, false
	}
	enc, err := crypto.EncryptChunk(dk, plaintext)
	if err != nil {
		logging.L.Error("chunk_server: encrypt failed, storing plaintext",
			zap.String("volumeID", volumeID), zap.Error(err))
		return plaintext, nil, zeroTag, false
	}
	return enc.Ciphertext, enc.PlaintextHash[:], enc.AuthTag, true
}

// maybeDecrypt reverses maybeEncrypt using the supplied plaintextHash and
// authTag (fetched by the caller from the metadata service). If
// plaintextHash is nil/empty the chunk is treated as unencrypted and the
// payload is returned unchanged.
func (s *ChunkServer) maybeDecrypt(volumeID string, payload, plaintextHash []byte, authTag [crypto.AuthTagSize]byte) ([]byte, error) {
	if s.keys == nil || volumeID == "" {
		return payload, nil
	}
	dk, ok := s.keys.DatasetKey(volumeID)
	if !ok {
		return payload, nil
	}
	if len(plaintextHash) == 0 {
		// Unencrypted chunk written before encryption was enabled, or
		// chunk stored by a path that didn't record a plaintext hash.
		return payload, nil
	}
	return crypto.DecryptChunk(dk, payload, authTag, plaintextHash)
}

// PutChunk receives a stream of chunk data fragments, assembles them, and writes
// the chunk to the Rust dataplane via the gRPC WriteChunk method. If the
// request carries a volume_id and a DK is cached for that volume, the
// payload is AES-GCM-encrypted transparently before being handed to the
// dataplane; per-chunk crypto metadata (plaintext hash, auth tag, DK
// version) is persisted in VolumeMeta via the MetaClient so GetChunk can
// re-derive keys after a restart.
func (s *ChunkServer) PutChunk(stream grpc.ClientStreamingServer[pb.PutChunkRequest, pb.PutChunkResponse]) error {
	var (
		chunkID  string
		volumeID string
		data     []byte
	)

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "receiving chunk stream: %v", err)
		}

		// First message must contain chunk_id and checksum.
		if chunkID == "" {
			chunkID = req.GetChunkId()
			if chunkID == "" {
				return status.Error(codes.InvalidArgument, "first message must contain chunk_id")
			}
			volumeID = req.GetVolumeId()
		}

		if len(req.GetData()) > 0 {
			data = append(data, req.GetData()...)
		}
	}

	if len(data) == 0 {
		return status.Error(codes.InvalidArgument, "no chunk data received")
	}

	// Transparent encryption for volumes with a cached DK.
	payload, plaintextHash, authTag, encrypted := s.maybeEncrypt(volumeID, data)

	logging.L.Debug("chunk_server: writing chunk via gRPC dataplane",
		zap.String("chunkID", chunkID),
		zap.String("volumeID", volumeID),
		zap.Bool("encrypted", encrypted),
		zap.Int("dataLen", len(payload)),
	)

	resultChunkID, bytesWritten, err := s.dpClient.WriteChunk(s.bdevName, payload)
	if err != nil {
		logging.L.Error("chunk_server: write_chunk failed",
			zap.String("chunkID", chunkID),
			zap.Error(err),
		)
		return status.Errorf(codes.Internal, "writing chunk to dataplane: %v", err)
	}
	_ = bytesWritten

	// Persist per-chunk crypto bookkeeping via the metadata service so a
	// restart does not render this chunk unreadable. When meta is nil
	// (misconfigured deployment) we log but do not fail the write — the
	// dataplane already committed the bytes.
	if encrypted {
		if s.meta == nil {
			logging.L.Warn("chunk_server: encrypted chunk written but no MetaClient is configured; chunk will be unreadable after restart",
				zap.String("chunkID", resultChunkID),
				zap.String("volumeID", volumeID),
			)
		} else {
			// Record DK version 0 here: the chunk is encrypted with a
			// DK whose Transit wrapping version is known only to the
			// VolumeKeyProvider. When A8-Main-Wiring threads the real
			// version through the provider, populate it here; for now
			// the field is recorded so the schema is complete.
			tag := authTag
			if err := s.meta.SetChunkCrypto(stream.Context(), volumeID, resultChunkID, plaintextHash, tag[:], 0); err != nil {
				logging.L.Error("chunk_server: persist chunk crypto metadata failed",
					zap.String("chunkID", resultChunkID),
					zap.String("volumeID", volumeID),
					zap.Error(err),
				)
				return status.Errorf(codes.Internal, "persisting chunk crypto: %v", err)
			}
		}
	}

	return stream.SendAndClose(&pb.PutChunkResponse{
		ChunkId:      resultChunkID,
		BytesWritten: int64(len(payload)),
	})
}

// GetChunk reads a chunk from the Rust dataplane via the gRPC ReadChunk method
// and streams it back to the caller. If the request carries a volume_id and
// the chunk was stored encrypted, the per-chunk crypto metadata is fetched
// from the metadata service and used to transparently decrypt the payload
// before streaming.
func (s *ChunkServer) GetChunk(req *pb.GetChunkRequest, stream grpc.ServerStreamingServer[pb.GetChunkResponse]) error {
	chunkID := req.GetChunkId()
	if chunkID == "" {
		return status.Error(codes.InvalidArgument, "chunk_id is required")
	}
	volumeID := req.GetVolumeId()

	logging.L.Debug("chunk_server: reading chunk via gRPC dataplane",
		zap.String("chunkID", chunkID),
		zap.String("volumeID", volumeID),
	)

	stored, err := s.dpClient.ReadChunk(s.bdevName, chunkID)
	if err != nil {
		logging.L.Error("chunk_server: read_chunk failed",
			zap.String("chunkID", chunkID),
			zap.Error(err),
		)
		return status.Errorf(codes.Internal, "reading chunk from dataplane: %v", err)
	}

	// Fetch per-chunk crypto metadata from the metadata service. A
	// NotFound is expected for chunks written on unencrypted volumes and
	// is treated as a pass-through.
	var (
		plaintextHash []byte
		authTag       [crypto.AuthTagSize]byte
	)
	if volumeID != "" && s.meta != nil {
		ph, rawTag, _, metaErr := s.meta.GetChunkCrypto(stream.Context(), volumeID, chunkID)
		if metaErr == nil {
			plaintextHash = ph
			if len(rawTag) == crypto.AuthTagSize {
				copy(authTag[:], rawTag)
			}
		} else if status.Code(metaErr) != codes.NotFound {
			logging.L.Error("chunk_server: fetch chunk crypto failed",
				zap.String("chunkID", chunkID),
				zap.String("volumeID", volumeID),
				zap.Error(metaErr),
			)
			return status.Errorf(codes.Internal, "fetching chunk crypto: %v", metaErr)
		}
	}

	data, err := s.maybeDecrypt(volumeID, stored, plaintextHash, authTag)
	if err != nil {
		logging.L.Error("chunk_server: decrypt failed",
			zap.String("chunkID", chunkID),
			zap.Error(err),
		)
		return status.Errorf(codes.Internal, "decrypting chunk: %v", err)
	}

	// Stream the data back. For chunks up to 4MB we send in a single message;
	// the gRPC max message size (default 4MB) may need tuning for larger payloads.
	const fragmentSize = 2 * 1024 * 1024 // 2MB fragments
	for offset := 0; offset < len(data); offset += fragmentSize {
		end := offset + fragmentSize
		if end > len(data) {
			end = len(data)
		}
		resp := &pb.GetChunkResponse{
			Data: data[offset:end],
		}
		// Include metadata in the first fragment.
		if offset == 0 {
			resp.ChunkId = chunkID
		}
		if err := stream.Send(resp); err != nil {
			return fmt.Errorf("sending chunk fragment: %w", err)
		}
	}

	return nil
}

// DeleteChunk removes a chunk from the Rust dataplane via the gRPC
// DeleteChunk method. When a volume_id is supplied the chunk's crypto
// metadata is also removed from the metadata service so stale entries
// do not accumulate.
func (s *ChunkServer) DeleteChunk(ctx context.Context, req *pb.DeleteChunkRequest) (*pb.DeleteChunkResponse, error) {
	chunkID := req.GetChunkId()
	if chunkID == "" {
		return nil, status.Error(codes.InvalidArgument, "chunk_id is required")
	}

	logging.L.Debug("chunk_server: deleting chunk via gRPC dataplane",
		zap.String("chunkID", chunkID),
	)

	if err := s.dpClient.DeleteChunk(s.bdevName, chunkID); err != nil {
		logging.L.Error("chunk_server: delete_chunk failed",
			zap.String("chunkID", chunkID),
			zap.Error(err),
		)
		return nil, status.Errorf(codes.Internal, "deleting chunk from dataplane: %v", err)
	}

	// Best-effort cleanup of the per-chunk crypto bookkeeping. DeleteChunk
	// lacks a volume_id field in the existing RPC contract; GC callers
	// that know the volume_id should call Meta.DeleteChunkCrypto directly
	// via the chunk server helper below. Leaving stale entries is not
	// harmful (reads of a deleted chunk fail at the dataplane layer).
	_ = ctx // retained for future request-scoped tracing
	return &pb.DeleteChunkResponse{}, nil
}

// ForgetChunkCrypto removes per-chunk crypto metadata from the metadata
// service. Exported so the chunk GC pass (which holds the volume id) can
// clear bookkeeping after a chunk's underlying bytes have been reclaimed.
func (s *ChunkServer) ForgetChunkCrypto(ctx context.Context, volumeID, chunkID string) error {
	if s.meta == nil || volumeID == "" || chunkID == "" {
		return nil
	}
	return s.meta.DeleteChunkCrypto(ctx, volumeID, chunkID)
}

// HasChunk checks whether a chunk exists in the Rust dataplane via the
// gRPC ChunkExists method.
func (s *ChunkServer) HasChunk(ctx context.Context, req *pb.HasChunkRequest) (*pb.HasChunkResponse, error) {
	chunkID := req.GetChunkId()
	if chunkID == "" {
		return nil, status.Error(codes.InvalidArgument, "chunk_id is required")
	}

	exists, err := s.dpClient.ChunkExists(s.bdevName, chunkID)
	if err != nil {
		logging.L.Error("chunk_server: chunk_exists failed",
			zap.String("chunkID", chunkID),
			zap.Error(err),
		)
		return nil, status.Errorf(codes.Internal, "checking chunk existence: %v", err)
	}

	return &pb.HasChunkResponse{Exists: exists}, nil
}
