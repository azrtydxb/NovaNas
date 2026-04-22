//! OpenBao Transit client (Rust mirror of
//! `storage/internal/openbao`). Only a trait + in-memory fake are
//! provided here — in production the dataplane receives already-
//! unwrapped Dataset Keys over gRPC from the Go agent, so a full HTTP
//! client is not strictly necessary on the dataplane. The trait and
//! fake exist for tests that exercise the full wrap/unwrap flow in
//! Rust.

use std::collections::HashMap;
use std::sync::Mutex;

use ring::aead::{Aad, LessSafeKey, Nonce, UnboundKey, AES_256_GCM};
use ring::rand::{SecureRandom, SystemRandom};

#[derive(Debug, thiserror::Error)]
pub enum TransitError {
    #[error("missing master key {0}")]
    MissingKey(String),
    #[error("missing master key version {version} for key {key}")]
    MissingVersion { key: String, version: u64 },
    #[error("malformed wrapped blob")]
    MalformedBlob,
    #[error("crypto failed")]
    CryptoFailed,
    #[error("rng failed")]
    RngFailed,
}

/// Minimal subset of the OpenBao Transit surface NovaNas uses.
pub trait TransitClient: Send + Sync {
    fn wrap_dk(&self, master_key_name: &str, raw_dk: &[u8])
        -> Result<(Vec<u8>, u64), TransitError>;
    fn unwrap_dk(&self, master_key_name: &str, wrapped: &[u8]) -> Result<Vec<u8>, TransitError>;
    fn rotate_master_key(&self, master_key_name: &str) -> Result<(), TransitError>;
    fn latest_version(&self, master_key_name: &str) -> Result<u64, TransitError>;
}

/// In-memory test double. Wraps under AES-256-GCM with a process-local
/// random master key, versioned just like real Transit.
pub struct FakeTransit {
    inner: Mutex<HashMap<String, FakeKey>>,
}

struct FakeKey {
    versions: HashMap<u64, [u8; 32]>,
    latest: u64,
}

impl Default for FakeTransit {
    fn default() -> Self {
        Self {
            inner: Mutex::new(HashMap::new()),
        }
    }
}

impl FakeTransit {
    pub fn new() -> Self {
        Self::default()
    }

    fn with_key<R>(
        &self,
        name: &str,
        f: impl FnOnce(&mut FakeKey) -> Result<R, TransitError>,
    ) -> Result<R, TransitError> {
        let mut g = self.inner.lock().unwrap();
        let entry = g.entry(name.to_string()).or_insert_with(|| FakeKey {
            versions: HashMap::new(),
            latest: 0,
        });
        if entry.latest == 0 {
            rotate_inner(entry)?;
        }
        f(entry)
    }
}

fn rotate_inner(k: &mut FakeKey) -> Result<(), TransitError> {
    let rng = SystemRandom::new();
    let mut mk = [0u8; 32];
    rng.fill(&mut mk).map_err(|_| TransitError::RngFailed)?;
    k.latest += 1;
    k.versions.insert(k.latest, mk);
    Ok(())
}

impl TransitClient for FakeTransit {
    fn wrap_dk(&self, name: &str, raw: &[u8]) -> Result<(Vec<u8>, u64), TransitError> {
        self.with_key(name, |k| {
            let mk = k
                .versions
                .get(&k.latest)
                .ok_or_else(|| TransitError::MissingVersion {
                    key: name.to_string(),
                    version: k.latest,
                })?;
            let unbound =
                UnboundKey::new(&AES_256_GCM, mk).map_err(|_| TransitError::CryptoFailed)?;
            let sealing = LessSafeKey::new(unbound);
            let rng = SystemRandom::new();
            let mut nonce_bytes = [0u8; 12];
            rng.fill(&mut nonce_bytes)
                .map_err(|_| TransitError::RngFailed)?;
            let nonce = Nonce::assume_unique_for_key(nonce_bytes);
            let mut in_out = raw.to_vec();
            let tag = sealing
                .seal_in_place_separate_tag(nonce, Aad::empty(), &mut in_out)
                .map_err(|_| TransitError::CryptoFailed)?;

            let mut blob = Vec::with_capacity(1 + 8 + 12 + in_out.len() + 16);
            blob.push(1); // version marker
            blob.extend_from_slice(&k.latest.to_be_bytes());
            blob.extend_from_slice(&nonce_bytes);
            blob.extend_from_slice(&in_out);
            blob.extend_from_slice(tag.as_ref());
            Ok((blob, k.latest))
        })
    }

    fn unwrap_dk(&self, name: &str, wrapped: &[u8]) -> Result<Vec<u8>, TransitError> {
        self.with_key(name, |k| {
            if wrapped.len() < 1 + 8 + 12 + 16 || wrapped[0] != 1 {
                return Err(TransitError::MalformedBlob);
            }
            let mut version_bytes = [0u8; 8];
            version_bytes.copy_from_slice(&wrapped[1..9]);
            let version = u64::from_be_bytes(version_bytes);
            let mk = k
                .versions
                .get(&version)
                .ok_or(TransitError::MissingVersion {
                    key: name.to_string(),
                    version,
                })?;
            let nonce_bytes: [u8; 12] = wrapped[9..21]
                .try_into()
                .map_err(|_| TransitError::MalformedBlob)?;
            let unbound =
                UnboundKey::new(&AES_256_GCM, mk).map_err(|_| TransitError::CryptoFailed)?;
            let opening = LessSafeKey::new(unbound);
            let nonce = Nonce::assume_unique_for_key(nonce_bytes);
            let mut combined = wrapped[21..].to_vec();
            let plaintext = opening
                .open_in_place(nonce, Aad::empty(), &mut combined)
                .map_err(|_| TransitError::CryptoFailed)?;
            Ok(plaintext.to_vec())
        })
    }

    fn rotate_master_key(&self, name: &str) -> Result<(), TransitError> {
        self.with_key(name, |k| rotate_inner(k))
    }

    fn latest_version(&self, name: &str) -> Result<u64, TransitError> {
        self.with_key(name, |k| Ok(k.latest))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn wrap_unwrap_roundtrip() {
        let f = FakeTransit::new();
        let dk = [0x11u8; 32];
        let (w, v) = f.wrap_dk("mk", &dk).unwrap();
        assert_eq!(v, 1);
        let got = f.unwrap_dk("mk", &w).unwrap();
        assert_eq!(got, dk);
    }

    #[test]
    fn rotation_preserves_old_wraps() {
        let f = FakeTransit::new();
        let dk = [0x22u8; 32];
        let (w1, v1) = f.wrap_dk("mk", &dk).unwrap();
        f.rotate_master_key("mk").unwrap();
        let (_, v2) = f.wrap_dk("mk", &dk).unwrap();
        assert_eq!(v2, v1 + 1);
        // Old wrap still unwraps.
        let got = f.unwrap_dk("mk", &w1).unwrap();
        assert_eq!(got, dk);
    }
}
