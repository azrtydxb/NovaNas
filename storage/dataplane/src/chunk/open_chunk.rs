//! Open-chunk lifecycle (A4-Metadata-As-Chunks).
//!
//! Mirrors `storage/internal/chunk/open_chunk.go`. Open chunks are
//! mutable, append-only, UUID-identified buffers that seal into
//! content-addressed chunks. They provide WAL-style low-latency
//! semantics on top of the 4 MiB content-addressed chunk substrate so
//! that the metadata service (BadgerDB) can run on the chunk engine
//! without waiting for a full chunk to fill before fsync.
//!
//! Lifecycle: Open -> (Append, Append, ...) -> (Full | Timeout) -> Seal.
//!
//! This module is deliberately pure Rust (no SPDK dependencies) so it
//! builds in CI without the data-plane feature set. The real replication
//! fan-out plugs into ChunkEngine and lives in engine.rs; this module
//! only provides the per-chunk state machine and an in-memory registry.

use ring::digest;
use std::collections::HashMap;
use std::sync::Mutex;
use std::time::{Duration, Instant};
use uuid::Uuid;

use crate::backend::chunk_store::CHUNK_SIZE;

/// Default capacity of an open chunk (64 KiB). Tunable per pool.
pub const DEFAULT_OPEN_CHUNK_CAPACITY: usize = 64 * 1024;

/// Default idle timeout before an open chunk is force-sealed (5 s).
pub const DEFAULT_OPEN_CHUNK_TIMEOUT: Duration = Duration::from_secs(5);

/// Identifier for an open (mutable) chunk.
///
/// This is distinct from the content-addressed `ChunkId` because an
/// open chunk's contents change with every append.
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct OpenChunkId(pub String);

impl OpenChunkId {
    /// Allocate a fresh random open-chunk id.
    pub fn new() -> Self {
        Self(Uuid::new_v4().simple().to_string())
    }
}

impl Default for OpenChunkId {
    fn default() -> Self {
        Self::new()
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum OpenChunkState {
    Open,
    Sealed,
}

/// Errors for the open-chunk lifecycle.
#[derive(Debug, thiserror::Error)]
pub enum OpenChunkError {
    #[error("open chunk is already sealed")]
    Sealed,
    #[error("open chunk is full (capacity {capacity} bytes)")]
    Full { capacity: usize },
    #[error("append offset {got} does not match current length {current}")]
    OffsetMismatch { got: usize, current: usize },
    #[error("open chunk not found: {0:?}")]
    NotFound(OpenChunkId),
    #[error("invalid capacity {0}: must be > 0 and <= {max}", max = CHUNK_SIZE)]
    InvalidCapacity(usize),
    #[error("chunk encryption failed")]
    EncryptFailed,
}

/// A single mutable, append-only open chunk.
#[derive(Debug)]
pub struct OpenChunk {
    id: OpenChunkId,
    pool_id: String,
    capacity: usize,
    buf: Vec<u8>,
    state: OpenChunkState,
    sealed_as: Option<[u8; 32]>,
    last_append: Instant,
}

impl OpenChunk {
    pub fn new(pool_id: impl Into<String>, capacity: usize) -> Result<Self, OpenChunkError> {
        if capacity == 0 || capacity > CHUNK_SIZE as usize {
            return Err(OpenChunkError::InvalidCapacity(capacity));
        }
        Ok(Self {
            id: OpenChunkId::new(),
            pool_id: pool_id.into(),
            capacity,
            buf: Vec::with_capacity(capacity),
            state: OpenChunkState::Open,
            sealed_as: None,
            last_append: Instant::now(),
        })
    }

    pub fn id(&self) -> &OpenChunkId {
        &self.id
    }

    pub fn pool_id(&self) -> &str {
        &self.pool_id
    }

    pub fn capacity(&self) -> usize {
        self.capacity
    }

    pub fn len(&self) -> usize {
        self.buf.len()
    }

    pub fn is_empty(&self) -> bool {
        self.buf.is_empty()
    }

    pub fn state(&self) -> OpenChunkState {
        self.state
    }

    pub fn sealed_as(&self) -> Option<&[u8; 32]> {
        self.sealed_as.as_ref()
    }

    /// Append data at the given offset; offset MUST equal current length.
    pub fn append(&mut self, offset: usize, data: &[u8]) -> Result<(), OpenChunkError> {
        if self.state == OpenChunkState::Sealed {
            return Err(OpenChunkError::Sealed);
        }
        if offset != self.buf.len() {
            return Err(OpenChunkError::OffsetMismatch {
                got: offset,
                current: self.buf.len(),
            });
        }
        if self.buf.len() + data.len() > self.capacity {
            return Err(OpenChunkError::Full {
                capacity: self.capacity,
            });
        }
        self.buf.extend_from_slice(data);
        self.last_append = Instant::now();
        Ok(())
    }

    /// Seal the chunk: compute SHA-256, freeze, return (sealed_id, data).
    pub fn seal(&mut self) -> Result<([u8; 32], Vec<u8>), OpenChunkError> {
        if self.state == OpenChunkState::Sealed {
            return Err(OpenChunkError::Sealed);
        }
        let digest_val = digest::digest(&digest::SHA256, &self.buf);
        let mut id = [0u8; 32];
        id.copy_from_slice(digest_val.as_ref());
        self.state = OpenChunkState::Sealed;
        self.sealed_as = Some(id);
        let out = std::mem::take(&mut self.buf);
        Ok((id, out))
    }

    /// Seal the chunk with convergent encryption under `dk` (32 bytes).
    /// Returns (chunk_id, ciphertext, auth_tag, plaintext_hash) where
    /// chunk_id = SHA-256(ciphertext || auth_tag). See
    /// `crate::crypto` for the scheme.
    pub fn seal_encrypted(
        &mut self,
        dk: &[u8],
    ) -> Result<([u8; 32], Vec<u8>, [u8; 16], [u8; 32]), OpenChunkError> {
        if self.state == OpenChunkState::Sealed {
            return Err(OpenChunkError::Sealed);
        }
        let enc = crate::crypto::encrypt_chunk(dk, &self.buf)
            .map_err(|_| OpenChunkError::EncryptFailed)?;
        self.state = OpenChunkState::Sealed;
        self.sealed_as = Some(enc.chunk_id);
        // Drop plaintext buffer.
        self.buf = Vec::new();
        Ok((
            enc.chunk_id,
            enc.ciphertext,
            enc.auth_tag,
            enc.plaintext_hash,
        ))
    }

    /// Whether this open chunk should be sealed now (full or idle past
    /// timeout). A zero timeout disables the idle check.
    pub fn should_seal(&self, timeout: Duration) -> bool {
        if self.state == OpenChunkState::Sealed {
            return false;
        }
        if self.buf.len() >= self.capacity {
            return true;
        }
        if !timeout.is_zero() && self.last_append.elapsed() >= timeout {
            return true;
        }
        false
    }
}

/// Replicator is the seam where CRUSH-driven fan-out is wired (#14).
///
/// On every `append`, the registry calls `replicate_append`; on
/// `seal`, `replicate_seal`. Implementations are responsible for:
///
///  - Picking replicas via CRUSH for the (pool_id, chunk_id) tuple.
///  - Fanning the operation to each replica synchronously — the
///    registry blocks the caller until acks return.
///  - Surfacing replica out-of-sync via the returned `Result`.
///
/// The registry uses a `NoopReplicator` when no replicator is set,
/// preserving the legacy single-node behavior. Production code wires
/// `GrpcReplicator` (lives in transport/, separate change).
pub trait Replicator: Send + Sync + std::fmt::Debug {
    /// Fan an append to replicas. Called *after* the local append
    /// succeeds; replication is best-effort if it fails the registry
    /// will surface the error and the caller can choose to roll back.
    fn replicate_append(
        &self,
        chunk: &OpenChunkId,
        offset: usize,
        data: &[u8],
    ) -> Result<(), OpenChunkError>;

    /// Fan a seal to replicas. ALL replicas must ack before the seal
    /// is considered complete (#14 acceptance: "all replicas must ack
    /// the seal before returning"). Returns Err on partial acks; the
    /// caller treats the seal as failed and the chunk stays open for
    /// retry.
    fn replicate_seal(&self, chunk: &OpenChunkId, sealed: &[u8]) -> Result<(), OpenChunkError>;
}

/// No-op Replicator: used in tests and single-node deployments.
#[derive(Debug, Default)]
pub struct NoopReplicator;

impl Replicator for NoopReplicator {
    fn replicate_append(
        &self,
        _chunk: &OpenChunkId,
        _offset: usize,
        _data: &[u8],
    ) -> Result<(), OpenChunkError> {
        Ok(())
    }

    fn replicate_seal(&self, _chunk: &OpenChunkId, _sealed: &[u8]) -> Result<(), OpenChunkError> {
        Ok(())
    }
}

/// In-memory registry of open chunks. Thread-safe via a single Mutex;
/// suitable for the modest number of concurrently-open chunks
/// (typically O(number of pools) for WAL traffic).
pub struct OpenChunkRegistry {
    inner: Mutex<HashMap<OpenChunkId, OpenChunk>>,
    replicator: Box<dyn Replicator>,
}

impl Default for OpenChunkRegistry {
    fn default() -> Self {
        Self {
            inner: Mutex::new(HashMap::new()),
            replicator: Box::new(NoopReplicator),
        }
    }
}

impl OpenChunkRegistry {
    pub fn new() -> Self {
        Self::default()
    }

    /// Build a registry with a custom Replicator (e.g. the gRPC
    /// fan-out used in multi-node deployments). The replicator is
    /// invoked synchronously inside `append` and `seal` so callers
    /// observe the same ordering as a single-node store.
    pub fn with_replicator(replicator: Box<dyn Replicator>) -> Self {
        Self {
            inner: Mutex::new(HashMap::new()),
            replicator,
        }
    }

    /// Allocate a new open chunk and return its id.
    pub fn open(
        &self,
        pool_id: impl Into<String>,
        capacity: usize,
    ) -> Result<OpenChunkId, OpenChunkError> {
        let c = OpenChunk::new(pool_id, capacity)?;
        let id = c.id().clone();
        self.inner.lock().unwrap().insert(id.clone(), c);
        Ok(id)
    }

    /// Append to the chunk with the given id, then fan the append
    /// out to replicas (#14). The fan-out is synchronous so the
    /// caller observes a single linearisable order — when this
    /// returns Ok the data is durable on every selected replica.
    pub fn append(
        &self,
        id: &OpenChunkId,
        offset: usize,
        data: &[u8],
    ) -> Result<usize, OpenChunkError> {
        let chunk_len = {
            let mut g = self.inner.lock().unwrap();
            let c = g
                .get_mut(id)
                .ok_or_else(|| OpenChunkError::NotFound(id.clone()))?;
            c.append(offset, data)?;
            c.len()
        };
        // Replicate outside the lock — replicator may issue gRPC.
        self.replicator.replicate_append(id, offset, data)?;
        Ok(chunk_len)
    }

    /// Seal the chunk locally, fan the seal out to replicas, and
    /// remove it from the registry once all replicas ack. If any
    /// replica fails to ack, the chunk is reinstated so the caller
    /// can retry (#14 acceptance).
    pub fn seal(&self, id: &OpenChunkId) -> Result<([u8; 32], Vec<u8>), OpenChunkError> {
        let mut c = {
            let mut g = self.inner.lock().unwrap();
            g.remove(id)
                .ok_or_else(|| OpenChunkError::NotFound(id.clone()))?
        };
        let result = c.seal()?;
        if let Err(err) = self.replicator.replicate_seal(id, &result.1) {
            // Roll back: the local seal is irreversible (state machine
            // only goes one way), but we put the chunk back so a retry
            // can re-issue the seal RPC against the same content.
            self.inner.lock().unwrap().insert(id.clone(), c);
            return Err(err);
        }
        Ok(result)
    }

    pub fn len(&self) -> usize {
        self.inner.lock().unwrap().len()
    }

    pub fn is_empty(&self) -> bool {
        self.inner.lock().unwrap().is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn append_and_seal_roundtrip() {
        let mut c = OpenChunk::new("pool-meta", 32).unwrap();
        c.append(0, b"hello ").unwrap();
        c.append(6, b"world").unwrap();
        assert_eq!(c.len(), 11);
        let (id, data) = c.seal().unwrap();
        assert_eq!(data, b"hello world".to_vec());
        // SHA-256 of "hello world".
        let expected = digest::digest(&digest::SHA256, b"hello world");
        assert_eq!(&id, <&[u8; 32]>::try_from(expected.as_ref()).unwrap());
        assert_eq!(c.state(), OpenChunkState::Sealed);
    }

    #[test]
    fn offset_mismatch_rejected() {
        let mut c = OpenChunk::new("p", 32).unwrap();
        c.append(0, b"abc").unwrap();
        match c.append(0, b"x") {
            Err(OpenChunkError::OffsetMismatch { got: 0, current: 3 }) => {}
            other => panic!("expected offset mismatch, got {:?}", other),
        }
    }

    #[test]
    fn full_rejected() {
        let mut c = OpenChunk::new("p", 4).unwrap();
        c.append(0, b"abcd").unwrap();
        assert!(matches!(
            c.append(4, b"e"),
            Err(OpenChunkError::Full { .. })
        ));
    }

    #[test]
    fn sealed_is_immutable() {
        let mut c = OpenChunk::new("p", 32).unwrap();
        c.append(0, b"data").unwrap();
        let _ = c.seal().unwrap();
        assert!(matches!(c.append(4, b"x"), Err(OpenChunkError::Sealed)));
        assert!(matches!(c.seal(), Err(OpenChunkError::Sealed)));
    }

    #[test]
    fn invalid_capacity() {
        assert!(matches!(
            OpenChunk::new("p", 0),
            Err(OpenChunkError::InvalidCapacity(0))
        ));
        assert!(matches!(
            OpenChunk::new("p", (CHUNK_SIZE as usize) + 1),
            Err(OpenChunkError::InvalidCapacity(_))
        ));
    }

    #[test]
    fn should_seal_full() {
        let mut c = OpenChunk::new("p", 4).unwrap();
        assert!(!c.should_seal(Duration::from_secs(60)));
        c.append(0, b"abcd").unwrap();
        assert!(c.should_seal(Duration::ZERO));
    }

    #[test]
    fn seal_encrypted_roundtrip() {
        let dk = [0x42u8; 32];
        let mut c = OpenChunk::new("p", 64).unwrap();
        c.append(0, b"hello").unwrap();
        c.append(5, b" world").unwrap();
        let (id, ct, tag, ph) = c.seal_encrypted(&dk).unwrap();
        assert_eq!(c.state(), OpenChunkState::Sealed);
        assert_ne!(
            ct,
            b"hello world".to_vec(),
            "ciphertext must differ from plaintext"
        );
        // id = SHA-256(ct || tag)
        let mut ctx = digest::Context::new(&digest::SHA256);
        ctx.update(&ct);
        ctx.update(&tag);
        let expected = ctx.finish();
        assert_eq!(&id[..], expected.as_ref());
        // Roundtrip.
        let got = crate::crypto::decrypt_chunk(&dk, &ct, &tag, &ph).unwrap();
        assert_eq!(got, b"hello world");
    }

    #[test]
    fn registry_lifecycle() {
        let reg = OpenChunkRegistry::new();
        let id = reg.open("pool", 64).unwrap();
        let after = reg.append(&id, 0, b"abc").unwrap();
        assert_eq!(after, 3);
        let after2 = reg.append(&id, 3, b"def").unwrap();
        assert_eq!(after2, 6);
        let (_, data) = reg.seal(&id).unwrap();
        assert_eq!(data, b"abcdef".to_vec());
        // After seal it must be gone.
        assert!(matches!(
            reg.append(&id, 0, b"x"),
            Err(OpenChunkError::NotFound(_))
        ));
    }
}
