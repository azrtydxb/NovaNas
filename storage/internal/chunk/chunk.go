// Package chunk provides chunk type definitions and utilities for the NovaStor
// storage system. All actual chunk I/O is handled by the Rust SPDK data-plane;
// this package provides only Go-side type definitions used by management-plane
// components (CSI controller, metadata, operator).
package chunk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc32"

	"github.com/azrtydxb/novanas/storage/internal/crypto"
	"github.com/azrtydxb/novanas/storage/internal/metadata"
)

// ChunkSize is the fixed size (4 MB) for all data chunks.
const ChunkSize = 4 * 1024 * 1024

// ChunkID uniquely identifies a chunk in storage via its SHA-256 hash.
type ChunkID string

// NewChunkID computes a content-addressed chunk ID from data using SHA-256.
// The same data always produces the same chunk ID, enabling deduplication.
func NewChunkID(data []byte) ChunkID {
	h := sha256.Sum256(data)
	return ChunkID(hex.EncodeToString(h[:]))
}

// Chunk represents a single unit of storage (4 MB) with its metadata.
// Chunks are immutable and content-addressed for deduplication.
//
// When Encrypted is true, Data holds the AES-256-GCM ciphertext (NOT
// plaintext); AuthTag carries the 16-byte GCM tag and PlaintextHash is
// the SHA-256 of the original plaintext — stored so that the chunk key
// can be re-derived at read time. The ChunkID on an encrypted chunk is
// SHA-256(ciphertext||auth_tag). See storage/internal/crypto for
// details.
type Chunk struct {
	ID       ChunkID
	Data     []byte
	Checksum uint32

	// Encryption fields (A4-Encryption, docs/02).
	Encrypted     bool
	AuthTag       [16]byte
	PlaintextHash [32]byte

	// ProtectionProfile specifies the data protection settings for this chunk.
	ProtectionProfile *metadata.ProtectionProfile `json:"protectionProfile,omitempty"`

	// ComplianceInfo tracks the current compliance state of this chunk.
	ComplianceInfo *metadata.ComplianceInfo `json:"complianceInfo,omitempty"`
}

// ComputeChecksum calculates the CRC-32C checksum of the chunk data.
func (c *Chunk) ComputeChecksum() uint32 {
	table := crc32.MakeTable(crc32.Castagnoli)
	return crc32.Checksum(c.Data, table)
}

// VerifyChecksum checks that the stored checksum matches the computed checksum.
// Returns an error if they differ, indicating data corruption.
func (c *Chunk) VerifyChecksum() error {
	actual := c.ComputeChecksum()
	if actual != c.Checksum {
		return fmt.Errorf("checksum mismatch for chunk %s: stored=%d computed=%d", c.ID, c.Checksum, actual)
	}
	return nil
}

// SplitData divides data into fixed-size 4 MB chunks.
// Each chunk is assigned a content-addressed ID and checksum.
// Empty input returns nil.
func SplitData(data []byte) []*Chunk {
	if len(data) == 0 {
		return nil
	}
	var chunks []*Chunk
	for offset := 0; offset < len(data); offset += ChunkSize {
		end := offset + ChunkSize
		if end > len(data) {
			end = len(data)
		}
		slice := data[offset:end]
		c := &Chunk{
			ID:   NewChunkID(slice),
			Data: slice,
		}
		c.Checksum = c.ComputeChecksum()
		chunks = append(chunks, c)
	}
	return chunks
}

// SplitDataEncrypted splits data into 4 MB chunks and encrypts each
// chunk under the supplied 32-byte dataset key using convergent
// AES-256-GCM. Each returned Chunk has Encrypted=true, Data holding
// the ciphertext, AuthTag set, PlaintextHash set, and ID equal to
// SHA-256(ciphertext||auth_tag).
//
// Convergence property: the same plaintext + same DK produces the
// same chunk id, so dedup still works within a DK scope. Different
// DKs produce different chunk ids for the same plaintext, so two
// volumes never cross-dedup.
//
// If dk is nil or empty this falls back to SplitData (unencrypted).
// Callers that want to assert encryption should validate dk
// themselves.
func SplitDataEncrypted(data []byte, dk []byte) ([]*Chunk, error) {
	if len(dk) == 0 {
		return SplitData(data), nil
	}
	if len(dk) != crypto.ChunkKeySize {
		return nil, fmt.Errorf("chunk: dataset key must be %d bytes, got %d", crypto.ChunkKeySize, len(dk))
	}
	if len(data) == 0 {
		return nil, nil
	}
	var chunks []*Chunk
	for offset := 0; offset < len(data); offset += ChunkSize {
		end := offset + ChunkSize
		if end > len(data) {
			end = len(data)
		}
		slice := data[offset:end]
		enc, err := crypto.EncryptChunk(dk, slice)
		if err != nil {
			return nil, fmt.Errorf("chunk: encrypt offset %d: %w", offset, err)
		}
		c := &Chunk{
			ID:            ChunkID(hex.EncodeToString(enc.ChunkID[:])),
			Data:          enc.Ciphertext,
			Encrypted:     true,
			AuthTag:       enc.AuthTag,
			PlaintextHash: enc.PlaintextHash,
		}
		c.Checksum = c.ComputeChecksum()
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// DecryptChunkData decrypts an encrypted Chunk under the supplied
// dataset key, returning the recovered plaintext. For non-encrypted
// chunks this is a no-op (returns Data directly).
func DecryptChunkData(c *Chunk, dk []byte) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("chunk: nil chunk")
	}
	if !c.Encrypted {
		out := make([]byte, len(c.Data))
		copy(out, c.Data)
		return out, nil
	}
	return crypto.DecryptChunk(dk, c.Data, c.AuthTag, c.PlaintextHash[:])
}
