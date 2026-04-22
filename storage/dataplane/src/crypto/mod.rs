//! Chunk-level convergent encryption (Rust mirror of
//! `storage/internal/crypto`). See that package's top-level doc
//! comment for the full scheme description; this module exports the
//! same primitives so the SPDK data-plane can seal, write, and read
//! encrypted chunks without round-tripping to the Go control plane.
//!
//! Key hierarchy:
//!   Master Key      — OpenBao Transit (never exported)
//!   Dataset Key     — per volume, unwrapped on mount
//!   Chunk Key       — HMAC-SHA-256(DK, "key"||plaintext_hash)
//!   IV              — HMAC-SHA-256(DK, "iv" ||plaintext_hash)[:12]
//!
//! Same (DK, plaintext) -> same ciphertext -> same chunk id: dedup
//! preserved within a DK scope.

use ring::aead::{Aad, LessSafeKey, Nonce, UnboundKey, AES_256_GCM};
use ring::digest::{self, SHA256};
use ring::hmac;

/// Dataset-key length (bytes).
pub const CHUNK_KEY_SIZE: usize = 32;

/// AES-GCM IV/nonce length (bytes).
pub const CHUNK_IV_SIZE: usize = 12;

/// AES-GCM authentication tag length (bytes).
pub const AUTH_TAG_SIZE: usize = 16;

/// SHA-256 output length (bytes).
pub const HASH_SIZE: usize = 32;

/// Domain-separation prefix for the chunk-key HMAC.
const KEY_DOMAIN: &[u8] = b"novanas/chunk-key/v1";

/// Domain-separation prefix for the IV HMAC.
const IV_DOMAIN: &[u8] = b"novanas/chunk-iv/v1";

/// Namespace distinguishing dedup semantics for a chunk id. The
/// default namespace is convergent (same plaintext + same DK -> same
/// id); SSEC is randomised per-chunk and never dedups.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Namespace {
    Default,
    Ssec,
}

/// Errors returned by the crypto primitives.
#[derive(Debug, thiserror::Error)]
pub enum CryptoError {
    #[error("dataset key must be {CHUNK_KEY_SIZE} bytes, got {0}")]
    BadKeyLength(usize),
    #[error("plaintext hash must be {HASH_SIZE} bytes, got {0}")]
    BadHashLength(usize),
    #[error("encrypt failed")]
    EncryptFailed,
    #[error("decrypt/authenticate failed")]
    DecryptFailed,
}

/// Derive the per-chunk AES-256 key and 96-bit IV from the dataset
/// key and the plaintext hash, using HMAC-SHA-256 with distinct
/// domain-separation prefixes.
pub fn derive_chunk_key(
    dk: &[u8],
    plaintext_hash: &[u8],
) -> Result<([u8; CHUNK_KEY_SIZE], [u8; CHUNK_IV_SIZE]), CryptoError> {
    if dk.len() != CHUNK_KEY_SIZE {
        return Err(CryptoError::BadKeyLength(dk.len()));
    }
    if plaintext_hash.len() != HASH_SIZE {
        return Err(CryptoError::BadHashLength(plaintext_hash.len()));
    }

    let hk = hmac::Key::new(hmac::HMAC_SHA256, dk);
    let mut mac_key = hmac::Context::with_key(&hk);
    mac_key.update(KEY_DOMAIN);
    mac_key.update(plaintext_hash);
    let tag_key = mac_key.sign();
    let mut key = [0u8; CHUNK_KEY_SIZE];
    key.copy_from_slice(&tag_key.as_ref()[..CHUNK_KEY_SIZE]);

    let mut mac_iv = hmac::Context::with_key(&hk);
    mac_iv.update(IV_DOMAIN);
    mac_iv.update(plaintext_hash);
    let tag_iv = mac_iv.sign();
    let mut iv = [0u8; CHUNK_IV_SIZE];
    iv.copy_from_slice(&tag_iv.as_ref()[..CHUNK_IV_SIZE]);

    Ok((key, iv))
}

/// SHA-256 of a plaintext buffer.
pub fn hash_plaintext(plaintext: &[u8]) -> [u8; HASH_SIZE] {
    let d = digest::digest(&SHA256, plaintext);
    let mut out = [0u8; HASH_SIZE];
    out.copy_from_slice(d.as_ref());
    out
}

/// Output of `encrypt_chunk`.
#[derive(Debug, Clone)]
pub struct EncryptedChunk {
    pub ciphertext: Vec<u8>,
    pub auth_tag: [u8; AUTH_TAG_SIZE],
    pub chunk_id: [u8; HASH_SIZE],
    pub plaintext_hash: [u8; HASH_SIZE],
    pub namespace: Namespace,
}

/// Encrypt plaintext under dk using convergent AES-256-GCM.
/// `chunk_id = SHA-256(ciphertext || auth_tag)`.
pub fn encrypt_chunk(dk: &[u8], plaintext: &[u8]) -> Result<EncryptedChunk, CryptoError> {
    let ph = hash_plaintext(plaintext);
    let (mut key, iv) = derive_chunk_key(dk, &ph)?;

    let unbound = UnboundKey::new(&AES_256_GCM, &key).map_err(|_| CryptoError::EncryptFailed)?;
    let sealing = LessSafeKey::new(unbound);
    let nonce = Nonce::assume_unique_for_key(iv);

    let mut in_out = plaintext.to_vec();
    let tag = sealing
        .seal_in_place_separate_tag(nonce, Aad::empty(), &mut in_out)
        .map_err(|_| CryptoError::EncryptFailed)?;

    let mut auth_tag = [0u8; AUTH_TAG_SIZE];
    auth_tag.copy_from_slice(tag.as_ref());

    // chunk id = SHA-256(ciphertext || tag)
    let mut ctx = digest::Context::new(&SHA256);
    ctx.update(&in_out);
    ctx.update(&auth_tag);
    let d = ctx.finish();
    let mut chunk_id = [0u8; HASH_SIZE];
    chunk_id.copy_from_slice(d.as_ref());

    // Zeroise the derived key.
    for b in key.iter_mut() {
        *b = 0;
    }

    Ok(EncryptedChunk {
        ciphertext: in_out,
        auth_tag,
        chunk_id,
        plaintext_hash: ph,
        namespace: Namespace::Default,
    })
}

/// Decrypt a chunk previously produced by `encrypt_chunk`. The caller
/// must supply the plaintext hash recorded in the chunk's metadata.
pub fn decrypt_chunk(
    dk: &[u8],
    ciphertext: &[u8],
    auth_tag: &[u8; AUTH_TAG_SIZE],
    plaintext_hash: &[u8],
) -> Result<Vec<u8>, CryptoError> {
    let (mut key, iv) = derive_chunk_key(dk, plaintext_hash)?;
    let unbound = UnboundKey::new(&AES_256_GCM, &key).map_err(|_| CryptoError::DecryptFailed)?;
    let opening = LessSafeKey::new(unbound);
    let nonce = Nonce::assume_unique_for_key(iv);

    let mut combined = Vec::with_capacity(ciphertext.len() + AUTH_TAG_SIZE);
    combined.extend_from_slice(ciphertext);
    combined.extend_from_slice(auth_tag);

    let plaintext = opening
        .open_in_place(nonce, Aad::empty(), &mut combined)
        .map_err(|_| CryptoError::DecryptFailed)?;
    let out = plaintext.to_vec();

    for b in key.iter_mut() {
        *b = 0;
    }
    Ok(out)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn rand_key() -> [u8; CHUNK_KEY_SIZE] {
        use ring::rand::SecureRandom;
        let rng = ring::rand::SystemRandom::new();
        let mut k = [0u8; CHUNK_KEY_SIZE];
        rng.fill(&mut k).unwrap();
        k
    }

    #[test]
    fn convergence_same_dk_and_plaintext() {
        let dk = rand_key();
        let pt = b"convergent test";
        let a = encrypt_chunk(&dk, pt).unwrap();
        let b = encrypt_chunk(&dk, pt).unwrap();
        assert_eq!(a.ciphertext, b.ciphertext);
        assert_eq!(a.chunk_id, b.chunk_id);
    }

    #[test]
    fn dedup_breaks_across_dk() {
        let dk1 = rand_key();
        let dk2 = rand_key();
        let pt = b"same plaintext";
        let a = encrypt_chunk(&dk1, pt).unwrap();
        let b = encrypt_chunk(&dk2, pt).unwrap();
        assert_ne!(a.chunk_id, b.chunk_id);
    }

    #[test]
    fn roundtrip() {
        let dk = rand_key();
        let pt = b"roundtrip plaintext";
        let enc = encrypt_chunk(&dk, pt).unwrap();
        let got = decrypt_chunk(&dk, &enc.ciphertext, &enc.auth_tag, &enc.plaintext_hash).unwrap();
        assert_eq!(got, pt);
    }

    #[test]
    fn tamper_detection_ciphertext() {
        let dk = rand_key();
        let enc = encrypt_chunk(&dk, b"data").unwrap();
        let mut ct = enc.ciphertext.clone();
        ct[0] ^= 0x01;
        assert!(decrypt_chunk(&dk, &ct, &enc.auth_tag, &enc.plaintext_hash).is_err());
    }

    #[test]
    fn tamper_detection_tag() {
        let dk = rand_key();
        let enc = encrypt_chunk(&dk, b"data").unwrap();
        let mut tag = enc.auth_tag;
        tag[0] ^= 0x01;
        assert!(decrypt_chunk(&dk, &enc.ciphertext, &tag, &enc.plaintext_hash).is_err());
    }

    #[test]
    fn wrong_dk_fails() {
        let dk1 = rand_key();
        let dk2 = rand_key();
        let enc = encrypt_chunk(&dk1, b"x").unwrap();
        assert!(decrypt_chunk(&dk2, &enc.ciphertext, &enc.auth_tag, &enc.plaintext_hash).is_err());
    }

    #[test]
    fn domain_separation() {
        let dk = rand_key();
        let h = hash_plaintext(b"p");
        let (k, iv) = derive_chunk_key(&dk, &h).unwrap();
        assert_ne!(&k[..CHUNK_IV_SIZE], &iv[..]);
    }

    #[test]
    fn bad_key_length_rejected() {
        let short = [0u8; 10];
        assert!(encrypt_chunk(&short, b"x").is_err());
    }
}
