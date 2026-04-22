package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// AuthTagSize is the AES-GCM authentication tag size (bytes).
const AuthTagSize = 16

// ChunkID is a content-addressed chunk identifier. For encrypted
// chunks, the raw bytes are SHA-256(ciphertext||auth_tag); for
// unencrypted chunks they are SHA-256(plaintext).
type ChunkID [sha256.Size]byte

// Hex renders the chunk id as lowercase hex — the canonical wire form.
func (id ChunkID) Hex() string { return hex.EncodeToString(id[:]) }

// Namespace distinguishes the dedup scope of a chunk id. Default
// chunks may dedup within a DK scope; SSEC chunks are explicitly
// non-dedup (each write produces a fresh id).
type Namespace int

const (
	// NamespaceDefault is the standard convergent namespace: same
	// (DK, plaintext) maps to the same chunk id.
	NamespaceDefault Namespace = iota
	// NamespaceSSEC is the SSE-C (customer-supplied-key) namespace:
	// per-chunk random IV, never dedups.
	NamespaceSSEC
)

// String renders the namespace prefix used when encoding chunk ids
// to external form (e.g. "ssec:<hex>" for SSEC).
func (n Namespace) String() string {
	switch n {
	case NamespaceDefault:
		return ""
	case NamespaceSSEC:
		return "ssec:"
	default:
		return fmt.Sprintf("ns%d:", int(n))
	}
}

// EncryptedChunk is the output of EncryptChunk: ciphertext + auth tag
// + derived chunk id + the plaintext hash (so the metadata layer can
// record it for later re-derivation of the chunk key on read).
type EncryptedChunk struct {
	Ciphertext    []byte
	AuthTag       [AuthTagSize]byte
	ChunkID       ChunkID
	PlaintextHash [sha256.Size]byte
	Namespace     Namespace
}

// EncryptChunk encrypts plaintext under dk using convergent AES-256-GCM
// in the default (dedup-capable) namespace. It returns the ciphertext,
// 16-byte auth tag, content-addressed chunk id, and the SHA-256 of the
// plaintext (which callers must persist in the volume's chunk index so
// that the chunk key can be re-derived at read time).
//
// Ciphertext is returned as a standalone buffer (not concatenated with
// the tag) because the data-plane stores the tag in a separate index
// field — matching how CRC32C is stored today.
func EncryptChunk(dk, plaintext []byte) (*EncryptedChunk, error) {
	if len(dk) != ChunkKeySize {
		return nil, fmt.Errorf("crypto: dataset key must be %d bytes, got %d", ChunkKeySize, len(dk))
	}
	plainHash := sha256.Sum256(plaintext)
	chunkKey, iv := DeriveChunkKey(dk, plainHash[:])

	block, err := aes.NewCipher(chunkKey[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: gcm: %w", err)
	}
	// Seal appends ciphertext||tag to the destination; slice the tag
	// off the end.
	sealed := gcm.Seal(nil, iv[:], plaintext, nil)
	if len(sealed) < AuthTagSize {
		return nil, errors.New("crypto: sealed output shorter than tag")
	}
	ctLen := len(sealed) - AuthTagSize
	ciphertext := sealed[:ctLen]
	var tag [AuthTagSize]byte
	copy(tag[:], sealed[ctLen:])

	// Chunk id = SHA-256(ciphertext || tag).
	h := sha256.New()
	h.Write(ciphertext)
	h.Write(tag[:])
	var id ChunkID
	copy(id[:], h.Sum(nil))

	// Zeroise the derived key slice.
	for i := range chunkKey {
		chunkKey[i] = 0
	}

	return &EncryptedChunk{
		Ciphertext:    ciphertext,
		AuthTag:       tag,
		ChunkID:       id,
		PlaintextHash: plainHash,
		Namespace:     NamespaceDefault,
	}, nil
}

// DecryptChunk reverses EncryptChunk. The caller must supply the
// plaintextHash stored when the chunk was written (used to re-derive
// the chunk key and IV). The AES-GCM tag check verifies ciphertext
// integrity; a tamper of any byte in ciphertext or tag fails the
// decrypt with an authentication error.
func DecryptChunk(dk, ciphertext []byte, authTag [AuthTagSize]byte, plaintextHash []byte) ([]byte, error) {
	if len(dk) != ChunkKeySize {
		return nil, fmt.Errorf("crypto: dataset key must be %d bytes, got %d", ChunkKeySize, len(dk))
	}
	if len(plaintextHash) != sha256.Size {
		return nil, fmt.Errorf("crypto: plaintext hash must be %d bytes, got %d", sha256.Size, len(plaintextHash))
	}
	chunkKey, iv := DeriveChunkKey(dk, plaintextHash)
	defer func() {
		for i := range chunkKey {
			chunkKey[i] = 0
		}
	}()

	block, err := aes.NewCipher(chunkKey[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: gcm: %w", err)
	}

	// Open expects ciphertext||tag concatenated.
	combined := make([]byte, 0, len(ciphertext)+AuthTagSize)
	combined = append(combined, ciphertext...)
	combined = append(combined, authTag[:]...)
	plaintext, err := gcm.Open(nil, iv[:], combined, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}

// EncryptChunkSSEC encrypts a chunk under a customer-supplied key in
// the non-dedup SSEC namespace. The IV is randomly generated per
// chunk (12 bytes from crypto/rand), so identical plaintexts produce
// independent ciphertexts. The chunk id is SHA-256(iv||ciphertext||tag)
// with a "ssec:" prefix to keep it out of the default dedup table.
//
// The returned EncryptedChunk has Namespace == NamespaceSSEC; callers
// must persist both the random IV and the tag to decrypt. In this
// namespace PlaintextHash is unused and left zero.
//
// NOTE: this is a forward-compatible primitive; full S3 SSE-C wiring
// in the gateway is TODO(wave-5).
func EncryptChunkSSEC(customerKey, plaintext []byte) (*EncryptedChunk, [ChunkIVSize]byte, error) {
	var iv [ChunkIVSize]byte
	if len(customerKey) != ChunkKeySize {
		return nil, iv, fmt.Errorf("crypto: SSEC customer key must be %d bytes, got %d", ChunkKeySize, len(customerKey))
	}
	if _, err := rand.Read(iv[:]); err != nil {
		return nil, iv, fmt.Errorf("crypto: ssec iv: %w", err)
	}
	block, err := aes.NewCipher(customerKey)
	if err != nil {
		return nil, iv, fmt.Errorf("crypto: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, iv, fmt.Errorf("crypto: gcm: %w", err)
	}
	sealed := gcm.Seal(nil, iv[:], plaintext, nil)
	ctLen := len(sealed) - AuthTagSize
	ciphertext := sealed[:ctLen]
	var tag [AuthTagSize]byte
	copy(tag[:], sealed[ctLen:])

	// Chunk id hashes iv||ciphertext||tag for namespace uniqueness.
	h := sha256.New()
	h.Write(iv[:])
	h.Write(ciphertext)
	h.Write(tag[:])
	var id ChunkID
	copy(id[:], h.Sum(nil))

	return &EncryptedChunk{
		Ciphertext: ciphertext,
		AuthTag:    tag,
		ChunkID:    id,
		Namespace:  NamespaceSSEC,
	}, iv, nil
}

// DecryptChunkSSEC reverses EncryptChunkSSEC.
func DecryptChunkSSEC(customerKey, ciphertext []byte, iv [ChunkIVSize]byte, authTag [AuthTagSize]byte) ([]byte, error) {
	if len(customerKey) != ChunkKeySize {
		return nil, fmt.Errorf("crypto: SSEC customer key must be %d bytes, got %d", ChunkKeySize, len(customerKey))
	}
	block, err := aes.NewCipher(customerKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: gcm: %w", err)
	}
	combined := make([]byte, 0, len(ciphertext)+AuthTagSize)
	combined = append(combined, ciphertext...)
	combined = append(combined, authTag[:]...)
	plaintext, err := gcm.Open(nil, iv[:], combined, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: ssec decrypt: %w", err)
	}
	return plaintext, nil
}
