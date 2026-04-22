// Package crypto implements NovaNas's chunk-level convergent encryption.
//
// # Scheme
//
// NovaNas encrypts chunk data with AES-256-GCM. Keys form a two-level
// hierarchy:
//
//	Master Key (MK)        lives in OpenBao Transit; never exported.
//	  └─ Dataset Key (DK)  per volume (BlockVolume / SharedFilesystem /
//	                       ObjectStore bucket); wrapped by MK, stored in the
//	                       volume's metadata in wrapped form; unwrapped on
//	                       mount and cached in memory in a KeyCache for the
//	                       lifetime of the mount.
//	     └─ Chunk Key      per chunk; derived deterministically via
//	                       HMAC-SHA-256(DK, "key"||plaintext_hash).
//	                       IV derived with domain separation:
//	                       HMAC-SHA-256(DK, "iv"||plaintext_hash)[:12].
//
// # Convergent encryption
//
// The chunk-key derivation is deterministic in (DK, plaintext_hash) so
// identical plaintext within the same DK produces identical ciphertext,
// preserving content-addressed deduplication within a DK's scope.
// Different DKs — i.e. different volumes — produce different ciphertexts
// for the same plaintext, so tenants' data does not cross-dedup.
//
// # Security tradeoff
//
// Convergent encryption leaks plaintext equality within the DK scope
// (an adversary with oracle access to the DK could detect "does chunk X
// equal known plaintext P"). This is the deliberate price paid for
// dedup. Per docs/02 decision S16/S17, volume-scoped DKs bound the
// leakage and tenants/workloads that require confidentiality from each
// other get distinct DKs automatically.
//
// # Chunk ID
//
//   - Unencrypted chunk:  ChunkID = SHA-256(plaintext)
//   - Encrypted chunk:    ChunkID = SHA-256(ciphertext || auth_tag)
//
// Because chunk-key derivation is deterministic, the ciphertext is too,
// so dedup over the ciphertext's hash still works.
//
// # Plaintext hash storage
//
// To derive the chunk key on read we need plaintext_hash; we cannot
// recompute it without decrypting. The plaintext_hash is therefore
// stored alongside the chunk id in the volume's chunk-list metadata (a
// 32-byte field per chunk). For unencrypted chunks this equals the
// chunk id; for encrypted chunks it is distinct.
//
// # SSE-C segregated namespace
//
// When an S3 client supplies its own key (SSE-C), we cannot derive keys
// from the volume DK. Such chunks are stored in a separate namespace
// that never participates in dedup — the IV is randomised and the
// chunk id carries an "ssec:" prefix to keep it out of the dedup table.
// Full SSE-C wiring in the S3 gateway is deferred to Wave 5.
//
// # Rotation
//
// OpenBao Transit rotation bumps the master key version. Existing
// wrapped DKs remain unwrappable via the version baked into the wrap
// blob. New wraps use the latest version. Chunk data is not
// re-encrypted on master rotation — only the DK wrapping changes.
// DK rotation is deliberately out of scope: it would require
// re-encrypting every chunk in a volume and breaks dedup.
//
// # References
//
//   - docs/02-storage-architecture.md — "Encryption" section
//   - docs/10-identity-and-secrets.md — OpenBao Transit usage
//   - docs/14-decision-log.md — decisions S16, S17, S18
package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
)

// ChunkKeySize is the length (bytes) of the derived AES-256-GCM chunk key.
const ChunkKeySize = 32

// ChunkIVSize is the length (bytes) of the AES-GCM IV (nonce).
const ChunkIVSize = 12

// Domain-separation prefixes for chunk-key and IV derivation. These
// ensure the two HMAC outputs cannot collide even if plaintext_hash
// happened to equal a prefix-stripped variant.
var (
	keyDomain = []byte("novanas/chunk-key/v1")
	ivDomain  = []byte("novanas/chunk-iv/v1")
)

// DeriveChunkKey derives the AES-256-GCM chunk key and 96-bit IV from
// the dataset key and the SHA-256 of the plaintext, using HMAC-SHA-256
// with distinct domain-separation prefixes. Same (dk, plaintextHash)
// always yields the same (key, iv): the basis of convergent encryption.
//
// dk must be exactly 32 bytes; plaintextHash must be exactly 32 bytes
// (SHA-256). The function panics on mismatch — callers that accept
// untrusted lengths must validate first.
func DeriveChunkKey(dk []byte, plaintextHash []byte) (key [ChunkKeySize]byte, iv [ChunkIVSize]byte) {
	if len(dk) != ChunkKeySize {
		panic("crypto: dataset key must be 32 bytes")
	}
	if len(plaintextHash) != sha256.Size {
		panic("crypto: plaintext hash must be 32 bytes")
	}

	macKey := hmac.New(sha256.New, dk)
	macKey.Write(keyDomain)
	macKey.Write(plaintextHash)
	sumKey := macKey.Sum(nil)
	copy(key[:], sumKey[:ChunkKeySize])

	macIV := hmac.New(sha256.New, dk)
	macIV.Write(ivDomain)
	macIV.Write(plaintextHash)
	sumIV := macIV.Sum(nil)
	copy(iv[:], sumIV[:ChunkIVSize])

	return key, iv
}

// HashPlaintext computes the SHA-256 of a plaintext buffer. Exposed as
// a named helper so callers do not scatter crypto/sha256 imports.
func HashPlaintext(plaintext []byte) [sha256.Size]byte {
	return sha256.Sum256(plaintext)
}
