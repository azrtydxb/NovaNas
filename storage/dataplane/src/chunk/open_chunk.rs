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

/// In-memory registry of open chunks. Thread-safe via a single Mutex;
/// suitable for the modest number of concurrently-open chunks
/// (typically O(number of pools) for WAL traffic).
#[derive(Default)]
pub struct OpenChunkRegistry {
    inner: Mutex<HashMap<OpenChunkId, OpenChunk>>,
}

impl OpenChunkRegistry {
    pub fn new() -> Self {
        Self::default()
    }

    /// Allocate a new open chunk and return its id.
    pub fn open(&self, pool_id: impl Into<String>, capacity: usize) -> Result<OpenChunkId, OpenChunkError> {
        let c = OpenChunk::new(pool_id, capacity)?;
        let id = c.id().clone();
        self.inner.lock().unwrap().insert(id.clone(), c);
        Ok(id)
    }

    /// Append to the chunk with the given id.
    pub fn append(&self, id: &OpenChunkId, offset: usize, data: &[u8]) -> Result<usize, OpenChunkError> {
        let mut g = self.inner.lock().unwrap();
        let c = g.get_mut(id).ok_or_else(|| OpenChunkError::NotFound(id.clone()))?;
        c.append(offset, data)?;
        Ok(c.len())
    }

    /// Seal the chunk and remove it from the registry.
    pub fn seal(&self, id: &OpenChunkId) -> Result<([u8; 32], Vec<u8>), OpenChunkError> {
        let mut g = self.inner.lock().unwrap();
        let mut c = g.remove(id).ok_or_else(|| OpenChunkError::NotFound(id.clone()))?;
        c.seal()
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
        assert!(matches!(c.append(4, b"e"), Err(OpenChunkError::Full { .. })));
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
        assert!(matches!(OpenChunk::new("p", 0), Err(OpenChunkError::InvalidCapacity(0))));
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
        assert!(matches!(reg.append(&id, 0, b"x"), Err(OpenChunkError::NotFound(_))));
    }
}
