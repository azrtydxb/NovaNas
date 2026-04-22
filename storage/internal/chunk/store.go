package chunk

import "context"

// Store is the core interface for chunk storage backends.
// In the current architecture, all chunk I/O is handled by the Rust SPDK
// data-plane via gRPC. This interface is retained for management-plane
// components (GC, datamover) that need a Go-side abstraction.
type Store interface {
	Put(ctx context.Context, c *Chunk) error
	Get(ctx context.Context, id ChunkID) (*Chunk, error)
	Delete(ctx context.Context, id ChunkID) error
	Has(ctx context.Context, id ChunkID) (bool, error)
	List(ctx context.Context) ([]ChunkID, error)
}

// StoreStats holds capacity and usage statistics for a storage backend.
type StoreStats struct {
	TotalBytes     int64
	UsedBytes      int64
	AvailableBytes int64
	ChunkCount     int64
}

// CapacityStore is an optional extension to Store for backends that can
// report their own storage capacity.
type CapacityStore interface {
	Store
	Stats(ctx context.Context) (*StoreStats, error)
}

// ChunkMeta holds chunk metadata without the full data payload.
type ChunkMeta struct {
	ID       ChunkID
	Size     int64
	Checksum uint32
}

// ChunkMetaStore is an optional extension for backends that can return
// chunk metadata (size, checksum) without loading the full data.
type ChunkMetaStore interface {
	Store
	GetMeta(ctx context.Context, id ChunkID) (*ChunkMeta, error)
}

// HealthCheckStore is an optional extension for backends that can
// report their own health.
type HealthCheckStore interface {
	Store
	HealthCheck(ctx context.Context) error
}

// OpenChunkStore is an optional extension for backends that implement the
// open-chunk lifecycle (mutable, append-only, UUID-identified chunks that
// seal to a content-addressed ChunkID). See open_chunk.go for the
// architectural rationale.
type OpenChunkStore interface {
	Store

	// OpenChunk allocates a new mutable chunk with the given capacity and
	// returns its UUID. poolID selects the CRUSH pool for placement.
	OpenChunk(ctx context.Context, poolID string, capacity int) (OpenChunkID, error)

	// AppendChunk appends data to an open chunk at the given offset.
	// offset MUST equal the chunk's current length (strict append-only).
	AppendChunk(ctx context.Context, id OpenChunkID, offset int, data []byte) error

	// SealChunk transitions the open chunk to immutable content-addressed
	// state and returns the resulting ChunkID (SHA-256 of the final data).
	SealChunk(ctx context.Context, id OpenChunkID) (ChunkID, error)
}
