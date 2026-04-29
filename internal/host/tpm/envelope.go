package tpm

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

// Envelope encryption: pair TPM-sealing (which caps at 128 bytes of
// plaintext per object) with AES-256-GCM so we can protect arbitrary-
// size payloads. We TPM-seal a fresh 32-byte AES key (the DEK), then
// AES-GCM encrypt the actual payload with the DEK.
//
// On-disk wire format:
//
//	2  bytes  sealedDEK length (uint16, big-endian)
//	N  bytes  TPM-sealed 32-byte AES-256 key
//	12 bytes  AES-GCM nonce
//	M  bytes  AES-GCM ciphertext + 16-byte tag
//
// Used by both nova-bao-unseal and nova-kdc-unseal so the two binaries
// stay byte-compatible and any future format bump is centralized.

// SealUnsealer is the minimal interface envelope encryption needs from
// a TPM. *Sealer satisfies it; tests pass a fake.
type SealUnsealer interface {
	Seal(plaintext []byte) ([]byte, error)
	Unseal(sealed []byte) ([]byte, error)
}

// WrapAEAD TPM-seals a fresh AES-256 DEK and AES-GCM-encrypts plaintext
// with it. Returns the on-disk wire format. Caller-supplied randomness
// is read from crypto/rand.
func WrapAEAD(plaintext []byte, sealer SealUnsealer) ([]byte, error) {
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return nil, fmt.Errorf("rand DEK: %w", err)
	}
	sealedDEK, err := sealer.Seal(dek)
	if err != nil {
		return nil, fmt.Errorf("tpm seal DEK: %w", err)
	}
	if len(sealedDEK) > 0xFFFF {
		return nil, fmt.Errorf("sealed DEK overflows uint16: %d bytes", len(sealedDEK))
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("rand nonce: %w", err)
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 0, 2+len(sealedDEK)+len(nonce)+len(ct))
	hdr := make([]byte, 2)
	binary.BigEndian.PutUint16(hdr, uint16(len(sealedDEK)))
	out = append(out, hdr...)
	out = append(out, sealedDEK...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// UnwrapAEAD reverses WrapAEAD: TPM-unseals the DEK, then AES-GCM
// decrypts the payload. Returns the plaintext.
//
// Returns ErrPCRMismatch (wrapped) when the TPM rejects the unseal
// because boot state has changed since wrap.
func UnwrapAEAD(blob []byte, sealer SealUnsealer) ([]byte, error) {
	if len(blob) < 2 {
		return nil, fmt.Errorf("envelope blob too short: %d bytes", len(blob))
	}
	dekLen := int(binary.BigEndian.Uint16(blob[:2]))
	if dekLen == 0 {
		return nil, fmt.Errorf("envelope blob: zero-length sealed DEK")
	}
	if len(blob) < 2+dekLen+12 {
		return nil, fmt.Errorf("envelope blob truncated: have %d, need >= %d",
			len(blob), 2+dekLen+12)
	}
	sealedDEK := blob[2 : 2+dekLen]
	dek, err := sealer.Unseal(sealedDEK)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes gcm: %w", err)
	}
	nonce := blob[2+dekLen : 2+dekLen+12]
	ct := blob[2+dekLen+12:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("aes-gcm open: %w", err)
	}
	return pt, nil
}
