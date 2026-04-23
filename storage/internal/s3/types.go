package s3

import "context"

// BucketInfo represents S3 bucket metadata.
type BucketInfo struct {
	Name         string
	CreationDate int64
	Versioning   string
	Owner        string
	MaxSize      int64 // Per-bucket quota limit in bytes (0 = unlimited)
}

// ObjectInfo represents S3 object metadata.
type ObjectInfo struct {
	Bucket      string
	Key         string
	Size        int64
	ETag        string
	ContentType string
	UserMeta    map[string]string
	ChunkIDs    []string
	VersionID   string
	ModTime     int64
}

// MultipartInfo represents a multipart upload.
type MultipartInfo struct {
	UploadID     string
	Bucket       string
	Key          string
	Parts        []PartInfo
	CreationDate int64
}

// PartInfo represents a single part of a multipart upload.
type PartInfo struct {
	PartNumber int
	Size       int64
	ETag       string
	ChunkIDs   []string
}

// BucketStore is the interface for bucket metadata operations.
type BucketStore interface {
	PutBucket(ctx context.Context, info *BucketInfo) error
	GetBucket(ctx context.Context, name string) (*BucketInfo, error)
	DeleteBucket(ctx context.Context, name string) error
	ListBuckets(ctx context.Context) ([]*BucketInfo, error)
}

// ObjectStore is the interface for object metadata operations.
type ObjectStore interface {
	PutObject(ctx context.Context, info *ObjectInfo) error
	GetObject(ctx context.Context, bucket, key string) (*ObjectInfo, error)
	DeleteObject(ctx context.Context, bucket, key string) error
	ListObjects(ctx context.Context, bucket, prefix string) ([]*ObjectInfo, error)
}

// ChunkStore is the interface for reading/writing chunk data.
type ChunkStore interface {
	PutChunkData(ctx context.Context, data []byte) (chunkID string, err error)
	GetChunkData(ctx context.Context, chunkID string) ([]byte, error)
	DeleteChunkData(ctx context.Context, chunkID string) error
}

// NamespacedChunkStore is the optional extension of ChunkStore that supports
// writing into an explicit content-addressed namespace. SSE-C requires the
// gateway to place ciphertext chunks into a "ssec" namespace that cannot
// dedup across requests (because each caller supplies their own key). The
// default namespace ("default") is what non-SSE-C and SSE-S3 writes target.
//
// Implementations that do not partition their address space may either:
//   - not implement this interface (the caller then falls back to
//     PutChunkData — fine for SSE-S3 / plaintext where dedup across callers
//     is safe), or
//   - implement it but ignore the namespace (DEGRADED: not recommended for
//     SSE-C production use because ciphertext from different caller keys
//     would share content-addressed IDs).
type NamespacedChunkStore interface {
	ChunkStore
	// PutChunkDataNS writes data into the given namespace. "default" means
	// the shared dedup namespace; any other value (e.g. "ssec") MUST be
	// stored in an isolated keyspace so content-addressed IDs collide only
	// within that namespace.
	PutChunkDataNS(ctx context.Context, namespace string, data []byte) (chunkID string, err error)
	// GetChunkDataNS reads data from the given namespace. It is an error
	// to read a chunk from a different namespace than the one it was
	// written into, as content-addressed IDs are namespace-scoped.
	GetChunkDataNS(ctx context.Context, namespace string, chunkID string) ([]byte, error)
}

// putChunkInNamespace writes data into the given namespace when the
// ChunkStore implementation supports NamespacedChunkStore, else falls back
// to the shared-dedup default path. Returns the chunkID (which MUST be
// prefixed with the namespace when non-default so later reads can route
// back to the correct keyspace).
func putChunkInNamespace(ctx context.Context, cs ChunkStore, namespace string, data []byte) (string, error) {
	if namespace == "" || namespace == "default" {
		return cs.PutChunkData(ctx, data)
	}
	ns, ok := cs.(NamespacedChunkStore)
	if !ok {
		// Fallback: store in the default namespace and tag the chunk ID
		// with the namespace prefix so reads still route correctly. Dedup
		// across SSE-C callers is IMPOSSIBLE in practice because each
		// caller's ciphertext differs under their customer key, so the
		// fallback is safe: identical plaintexts under different customer
		// keys produce different ciphertext and therefore different IDs.
		id, err := cs.PutChunkData(ctx, data)
		if err != nil {
			return "", err
		}
		return namespace + ":" + id, nil
	}
	id, err := ns.PutChunkDataNS(ctx, namespace, data)
	if err != nil {
		return "", err
	}
	return namespace + ":" + id, nil
}

// getChunkFromNamespace reads data from the namespace-prefixed chunk ID
// produced by putChunkInNamespace. Plain (unprefixed) IDs go to the shared
// default namespace.
func getChunkFromNamespace(ctx context.Context, cs ChunkStore, chunkID string) ([]byte, error) {
	// Parse "<ns>:<id>" form. We only recognise known namespace prefixes
	// so a legitimate chunk ID that happens to contain a colon (base64
	// etc.) is not misinterpreted.
	for _, prefix := range []string{"ssec:"} {
		if len(chunkID) > len(prefix) && chunkID[:len(prefix)] == prefix {
			id := chunkID[len(prefix):]
			ns, ok := cs.(NamespacedChunkStore)
			if ok {
				return ns.GetChunkDataNS(ctx, prefix[:len(prefix)-1], id)
			}
			return cs.GetChunkData(ctx, id)
		}
	}
	return cs.GetChunkData(ctx, chunkID)
}

// MultipartStore is the interface for multipart upload metadata.
type MultipartStore interface {
	PutMultipart(ctx context.Context, info *MultipartInfo) error
	GetMultipart(ctx context.Context, uploadID string) (*MultipartInfo, error)
	DeleteMultipart(ctx context.Context, uploadID string) error
}

// QuotaChecker defines the interface for checking storage quotas.
type QuotaChecker interface {
	// CheckStorageQuota checks if a storage allocation would exceed the quota.
	CheckStorageQuota(ctx context.Context, scope string, requestedBytes int64) error
	// ReserveStorage reserves storage capacity for a scope.
	ReserveStorage(ctx context.Context, scope string, bytes int64) error
	// ReleaseStorage releases storage capacity for a scope.
	ReleaseStorage(ctx context.Context, scope string, bytes int64) error
	// GetBucketUsage returns the current usage for a bucket.
	GetBucketUsage(ctx context.Context, bucket string) (int64, error)
}
