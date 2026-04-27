//! Open-chunk lifecycle (ported from `dataplane/src/chunk/open_chunk.rs`).
//!
//! Open chunks are mutable, append-only, UUID-identified buffers that seal
//! into content-addressed chunks. They give the frontend a WAL-style
//! low-latency primitive on top of the 4 MiB content-addressed chunk
//! substrate.
//!
//! This is the lean, frontend-only flavour: the dataplane variant carried
//! convergent encryption and pulled in `chunk_store::CHUNK_SIZE`. The
//! frontend doesn't encrypt locally (data does, transparently), and the
//! chunk size is a constant of the system, so we hard-code 4 MiB here and
//! drop the crypto seam.
//!
//! Lifecycle: Open -> (Append, Append, ...) -> (Full | Timeout) -> Seal.

use ring::digest;
use std::collections::HashMap;
use std::sync::Mutex;
use std::time::{Duration, Instant};
use uuid::Uuid;

/// Maximum chunk size — matches the data daemon's `CHUNK_SIZE` constant
/// (4 MiB). Hard-coded here so this module has zero deps on dataplane.
pub const CHUNK_SIZE: u64 = 4 * 1024 * 1024;

/// Default capacity of an open chunk (64 KiB). Tunable per pool.
pub const DEFAULT_OPEN_CHUNK_CAPACITY: usize = 64 * 1024;

/// Default idle timeout before an open chunk is force-sealed (5 s).
pub const DEFAULT_OPEN_CHUNK_TIMEOUT: Duration = Duration::from_secs(5);

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct OpenChunkId(pub String);

impl OpenChunkId {
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
        if capacity == 0 || capacity as u64 > CHUNK_SIZE {
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

/// Replicator seam — kept for API parity with the dataplane variant.
///
/// In the frontend, replication is not the frontend's concern: the data
/// daemon owns cross-disk replication. So the production wiring is the
/// `NoopReplicator`. The trait remains so test code that wants to inject
/// a fake replicator (e.g. to assert seal-fan-out semantics) can do so.
pub trait Replicator: Send + Sync + std::fmt::Debug {
    fn replicate_append(
        &self,
        chunk: &OpenChunkId,
        offset: usize,
        data: &[u8],
    ) -> Result<(), OpenChunkError>;
    fn replicate_seal(&self, chunk: &OpenChunkId, sealed: &[u8]) -> Result<(), OpenChunkError>;
}

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

    pub fn with_replicator(replicator: Box<dyn Replicator>) -> Self {
        Self {
            inner: Mutex::new(HashMap::new()),
            replicator,
        }
    }

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
        self.replicator.replicate_append(id, offset, data)?;
        Ok(chunk_len)
    }

    pub fn seal(&self, id: &OpenChunkId) -> Result<([u8; 32], Vec<u8>), OpenChunkError> {
        let mut c = {
            let mut g = self.inner.lock().unwrap();
            g.remove(id)
                .ok_or_else(|| OpenChunkError::NotFound(id.clone()))?
        };
        let result = c.seal()?;
        if let Err(err) = self.replicator.replicate_seal(id, &result.1) {
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
    fn registry_lifecycle() {
        let reg = OpenChunkRegistry::new();
        let id = reg.open("pool", 64).unwrap();
        let after = reg.append(&id, 0, b"abc").unwrap();
        assert_eq!(after, 3);
        let after2 = reg.append(&id, 3, b"def").unwrap();
        assert_eq!(after2, 6);
        let (_, data) = reg.seal(&id).unwrap();
        assert_eq!(data, b"abcdef".to_vec());
        assert!(matches!(
            reg.append(&id, 0, b"x"),
            Err(OpenChunkError::NotFound(_))
        ));
    }

    #[derive(Debug, Default)]
    struct CountingReplicator {
        appends: std::sync::Mutex<u32>,
        seals: std::sync::Mutex<u32>,
    }

    impl Replicator for CountingReplicator {
        fn replicate_append(
            &self,
            _: &OpenChunkId,
            _: usize,
            _: &[u8],
        ) -> Result<(), OpenChunkError> {
            *self.appends.lock().unwrap() += 1;
            Ok(())
        }
        fn replicate_seal(&self, _: &OpenChunkId, _: &[u8]) -> Result<(), OpenChunkError> {
            *self.seals.lock().unwrap() += 1;
            Ok(())
        }
    }

    #[test]
    fn registry_invokes_replicator() {
        let counter = std::sync::Arc::new(CountingReplicator::default());
        // Box::leak alternative: use a dedicated wrapper that holds Arc.
        #[derive(Debug)]
        struct Wrap(std::sync::Arc<CountingReplicator>);
        impl Replicator for Wrap {
            fn replicate_append(
                &self,
                c: &OpenChunkId,
                o: usize,
                d: &[u8],
            ) -> Result<(), OpenChunkError> {
                self.0.replicate_append(c, o, d)
            }
            fn replicate_seal(&self, c: &OpenChunkId, s: &[u8]) -> Result<(), OpenChunkError> {
                self.0.replicate_seal(c, s)
            }
        }

        let reg = OpenChunkRegistry::with_replicator(Box::new(Wrap(counter.clone())));
        let id = reg.open("p", 64).unwrap();
        reg.append(&id, 0, b"abc").unwrap();
        reg.append(&id, 3, b"def").unwrap();
        reg.seal(&id).unwrap();
        assert_eq!(*counter.appends.lock().unwrap(), 2);
        assert_eq!(*counter.seals.lock().unwrap(), 1);
    }
}
