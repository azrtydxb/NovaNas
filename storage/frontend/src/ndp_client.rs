//! Chunk-keyed NDP client.
//!
//! The frontend talks to the local data daemon over a Unix domain socket
//! using the existing NDP framing crate (`novanas-ndp`). NDP's wire form
//! is a 64-byte header + optional payload; the original ops are
//! volume-hash + offset based (legacy) but for the new architecture we
//! treat the chunk_id as the addressing key.
//!
//! Convention used here:
//!   - PUT chunk: NdpOp::Write with `volume_hash` set to a 64-bit hash
//!     of the chunk_id, `offset` = 0, and the chunk_id sent as a UTF-8
//!     prefix in the payload (`<chunk_id>\n<chunk_with_header>`).
//!   - GET chunk: NdpOp::Read with the chunk_id hash in `volume_hash`,
//!     offset 0, and the request payload carrying the chunk_id text.
//!
//! That choice is deliberate: it lets the frontend ship today against
//! the unchanged NDP wire format. Agent C's data refactor can either
//! adopt this convention verbatim or evolve NDP with a chunk-id-native
//! op; the trait abstraction below means swapping is mechanical.

use std::sync::Arc;

use async_trait::async_trait;
use ndp::{NdpConnection, NdpHeader, NdpOp};
use tokio::sync::Mutex;

use crate::error::{FrontendError, Result};

/// Trait abstracting chunk-keyed I/O against data. Implementations:
///   - `UdsNdpClient` — production: NDP over a Unix socket
///   - `tests::FakeNdp` — test double; in-memory map
#[async_trait]
pub trait NdpChunkClient: Send + Sync {
    async fn put_chunk(&self, chunk_id: &str, payload_with_header: &[u8]) -> Result<()>;
    async fn get_chunk(&self, chunk_id: &str) -> Result<Vec<u8>>;
    async fn delete_chunk(&self, chunk_id: &str) -> Result<()>;
}

/// Hash a chunk_id into the 64-bit volume_hash slot of an NDP header.
fn chunk_hash(chunk_id: &str) -> u64 {
    ndp::header::volume_hash(chunk_id)
}

/// Production NDP client: lazy-connects to a Unix socket.
pub struct UdsNdpClient {
    socket_path: String,
    conn: Mutex<Option<Arc<NdpConnection>>>,
}

impl UdsNdpClient {
    pub fn new(socket_path: impl Into<String>) -> Self {
        Self {
            socket_path: socket_path.into(),
            conn: Mutex::new(None),
        }
    }

    async fn conn(&self) -> Result<Arc<NdpConnection>> {
        let mut guard = self.conn.lock().await;
        if let Some(c) = guard.as_ref() {
            return Ok(c.clone());
        }
        let c = NdpConnection::connect_unix(&self.socket_path)
            .await
            .map_err(|e| FrontendError::Ndp(format!("connect {}: {}", self.socket_path, e)))?;
        let arc = Arc::new(c);
        *guard = Some(arc.clone());
        Ok(arc)
    }

    async fn invalidate(&self) {
        *self.conn.lock().await = None;
    }
}

#[async_trait]
impl NdpChunkClient for UdsNdpClient {
    async fn put_chunk(&self, chunk_id: &str, payload_with_header: &[u8]) -> Result<()> {
        let mut payload = Vec::with_capacity(chunk_id.len() + 1 + payload_with_header.len());
        payload.extend_from_slice(chunk_id.as_bytes());
        payload.push(b'\n');
        payload.extend_from_slice(payload_with_header);

        let header = NdpHeader::request(
            NdpOp::Write,
            0,
            chunk_hash(chunk_id),
            0,
            payload.len() as u32,
        );

        let conn = self.conn().await?;
        let resp = match conn.request(header, Some(payload.clone())).await {
            Ok(r) => r,
            Err(_) => {
                self.invalidate().await;
                let conn2 = self.conn().await?;
                conn2
                    .request(header, Some(payload))
                    .await
                    .map_err(|e| FrontendError::Ndp(format!("PutChunk retry: {}", e)))?
            }
        };
        if resp.header.status != 0 {
            return Err(FrontendError::Ndp(format!(
                "PutChunk status={}",
                resp.header.status
            )));
        }
        Ok(())
    }

    async fn get_chunk(&self, chunk_id: &str) -> Result<Vec<u8>> {
        // Send chunk_id as request payload and a Read header.
        let payload = chunk_id.as_bytes().to_vec();
        let header = NdpHeader::request(
            NdpOp::Read,
            0,
            chunk_hash(chunk_id),
            0,
            payload.len() as u32,
        );
        let conn = self.conn().await?;
        let resp = match conn.request(header, Some(payload.clone())).await {
            Ok(r) => r,
            Err(_) => {
                self.invalidate().await;
                let conn2 = self.conn().await?;
                conn2
                    .request(header, Some(payload))
                    .await
                    .map_err(|e| FrontendError::Ndp(format!("GetChunk retry: {}", e)))?
            }
        };
        if resp.header.status != 0 {
            return Err(FrontendError::Ndp(format!(
                "GetChunk status={}",
                resp.header.status
            )));
        }
        resp.data
            .ok_or_else(|| FrontendError::Ndp("GetChunk: empty response payload".into()))
    }

    async fn delete_chunk(&self, chunk_id: &str) -> Result<()> {
        // Reuse Unmap as the "delete chunk" op. Agent C may evolve this.
        let payload = chunk_id.as_bytes().to_vec();
        let header = NdpHeader::request(
            NdpOp::Unmap,
            0,
            chunk_hash(chunk_id),
            0,
            payload.len() as u32,
        );
        let conn = self.conn().await?;
        let resp = conn
            .request(header, Some(payload))
            .await
            .map_err(|e| FrontendError::Ndp(format!("DeleteChunk: {}", e)))?;
        if resp.header.status != 0 {
            return Err(FrontendError::Ndp(format!(
                "DeleteChunk status={}",
                resp.header.status
            )));
        }
        Ok(())
    }
}

#[cfg(test)]
pub(crate) mod tests {
    use super::*;
    use std::collections::HashMap;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::sync::Mutex as StdMutex;

    /// In-memory NDP double for unit + integration tests.
    pub struct FakeNdp {
        store: StdMutex<HashMap<String, Vec<u8>>>,
        puts: AtomicUsize,
        gets: AtomicUsize,
    }

    impl FakeNdp {
        pub fn new() -> Self {
            Self {
                store: StdMutex::new(HashMap::new()),
                puts: AtomicUsize::new(0),
                gets: AtomicUsize::new(0),
            }
        }
        pub fn put_count(&self) -> usize {
            self.puts.load(Ordering::Relaxed)
        }
        #[allow(dead_code)]
        pub fn get_count(&self) -> usize {
            self.gets.load(Ordering::Relaxed)
        }
        pub fn has(&self, chunk_id: &str) -> bool {
            self.store.lock().unwrap().contains_key(chunk_id)
        }
    }

    #[async_trait]
    impl NdpChunkClient for FakeNdp {
        async fn put_chunk(&self, chunk_id: &str, payload: &[u8]) -> Result<()> {
            self.store
                .lock()
                .unwrap()
                .insert(chunk_id.to_string(), payload.to_vec());
            self.puts.fetch_add(1, Ordering::Relaxed);
            Ok(())
        }
        async fn get_chunk(&self, chunk_id: &str) -> Result<Vec<u8>> {
            self.gets.fetch_add(1, Ordering::Relaxed);
            self.store
                .lock()
                .unwrap()
                .get(chunk_id)
                .cloned()
                .ok_or_else(|| FrontendError::Ndp(format!("not found: {}", chunk_id)))
        }
        async fn delete_chunk(&self, chunk_id: &str) -> Result<()> {
            self.store.lock().unwrap().remove(chunk_id);
            Ok(())
        }
    }

    #[test]
    fn chunk_hash_is_deterministic() {
        let h1 = chunk_hash("abc");
        let h2 = chunk_hash("abc");
        assert_eq!(h1, h2);
        assert_ne!(chunk_hash("abc"), chunk_hash("abd"));
    }

    #[tokio::test]
    async fn fake_ndp_roundtrips() {
        let fake = FakeNdp::new();
        fake.put_chunk("cid1", b"hello").await.unwrap();
        assert_eq!(fake.put_count(), 1);
        let got = fake.get_chunk("cid1").await.unwrap();
        assert_eq!(got, b"hello");
        assert!(fake.has("cid1"));
        fake.delete_chunk("cid1").await.unwrap();
        assert!(!fake.has("cid1"));
    }
}
