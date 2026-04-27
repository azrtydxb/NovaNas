//! Chunk-level operations for the policy engine.
//!
//! NovaNas is single-node by design (docs/14 S9, S12): "replication" means
//! multiple copies across *devices* (bdevs) on the single host, not across
//! peer nodes. All chunk I/O therefore goes through the local `ChunkStore`
//! trait — the previous gRPC `ChunkClient` remote branch has been removed.

use std::sync::Arc;

use crate::backend::chunk_store::ChunkStore;
use crate::error::Result;

/// Executes chunk-level operations for the policy engine.
///
/// Every target on a single-node cluster is the local node; the
/// `local_node_id` field is retained for bookkeeping and to reject any
/// action whose target drifts away from the local node.
pub struct ChunkOperations {
    local_node_id: String,
    local_store: Arc<dyn ChunkStore>,
}

impl ChunkOperations {
    pub fn new(local_node_id: String, local_store: Arc<dyn ChunkStore>) -> Self {
        Self {
            local_node_id,
            local_store,
        }
    }

    fn ensure_local(&self, node_id: &str) -> Result<()> {
        if node_id != self.local_node_id {
            return Err(crate::error::DataPlaneError::ChunkEngineError(format!(
                "unexpected non-local placement target {} on single-node deployment",
                node_id
            )));
        }
        Ok(())
    }

    /// Replicate a chunk from one node to another.
    ///
    /// On single-node both endpoints are the local store; the operation
    /// degenerates to a read-then-write through the same backend and is
    /// idempotent.
    pub async fn replicate_chunk(
        &self,
        chunk_id: &str,
        source_node_id: &str,
        target_node_id: &str,
    ) -> Result<()> {
        self.ensure_local(source_node_id)?;
        self.ensure_local(target_node_id)?;
        let data = self.local_store.get(chunk_id).await?;
        self.local_store.put(chunk_id, &data).await?;
        Ok(())
    }

    /// Reconstruct a missing EC shard from surviving shards using
    /// Reed-Solomon decoding, then write the result to the target.
    ///
    /// On single-node every shard lives on some local backend; `surviving`
    /// carries `(shard_index, node_id)` pairs and every read goes through
    /// the local store.
    pub async fn reconstruct_shard(
        &self,
        chunk_id: &str,
        shard_index: usize,
        data_shards: u32,
        parity_shards: u32,
        surviving: &[(usize, String)],
        target_node_id: &str,
    ) -> Result<()> {
        use crate::backend::chunk_store::ChunkHeader;

        self.ensure_local(target_node_id)?;

        // Read all surviving shards from the local store.
        let mut available: Vec<(usize, Vec<u8>)> = Vec::new();
        for (idx, node_id) in surviving {
            self.ensure_local(node_id)?;
            let shard_id = format!("{chunk_id}:shard:{idx}");
            let shard_with_header = self.local_store.get(&shard_id).await?;
            if shard_with_header.len() < ChunkHeader::SIZE {
                continue;
            }
            let header_bytes: [u8; ChunkHeader::SIZE] = shard_with_header[..ChunkHeader::SIZE]
                .try_into()
                .map_err(|_| {
                    crate::error::DataPlaneError::ChunkEngineError(
                        "shard header read failed".into(),
                    )
                })?;
            let header = ChunkHeader::from_bytes(&header_bytes)?;
            let data_len = header.data_len as usize;
            if shard_with_header.len() < ChunkHeader::SIZE + data_len {
                continue;
            }
            let raw = shard_with_header[ChunkHeader::SIZE..ChunkHeader::SIZE + data_len].to_vec();
            available.push((*idx, raw));
        }

        if available.len() < data_shards as usize {
            return Err(crate::error::DataPlaneError::ChunkEngineError(format!(
                "insufficient surviving shards for reconstruction: have {}, need {}",
                available.len(),
                data_shards
            )));
        }

        let mut originals: Vec<(usize, &[u8])> = Vec::new();
        let mut recovery: Vec<(usize, &[u8])> = Vec::new();
        for (idx, data) in &available {
            if *idx < data_shards as usize {
                originals.push((*idx, data.as_slice()));
            } else {
                let recovery_idx = *idx - data_shards as usize;
                recovery.push((recovery_idx, data.as_slice()));
            }
        }

        let reconstructed_shard = if shard_index < data_shards as usize {
            let recovered = reed_solomon_simd::decode(
                data_shards as usize,
                parity_shards as usize,
                originals,
                recovery,
            )
            .map_err(|e| {
                crate::error::DataPlaneError::ChunkEngineError(format!("RS decode failed: {e}"))
            })?;
            recovered.get(&shard_index).cloned().ok_or_else(|| {
                crate::error::DataPlaneError::ChunkEngineError(format!(
                    "RS decode did not produce shard {shard_index}"
                ))
            })?
        } else {
            let mut data_pieces: Vec<Vec<u8>> = vec![Vec::new(); data_shards as usize];
            for (idx, data) in &available {
                if *idx < data_shards as usize {
                    data_pieces[*idx] = data.clone();
                }
            }
            let all_data_present = data_pieces.iter().all(|d| !d.is_empty());
            if !all_data_present {
                return Err(crate::error::DataPlaneError::ChunkEngineError(
                    "cannot reconstruct parity: not all data shards available".into(),
                ));
            }
            let data_refs: Vec<&[u8]> = data_pieces.iter().map(|d| d.as_slice()).collect();
            let parity = reed_solomon_simd::encode(
                data_shards as usize,
                parity_shards as usize,
                data_refs.into_iter(),
            )
            .map_err(|e| {
                crate::error::DataPlaneError::ChunkEngineError(format!("RS encode failed: {e}"))
            })?;
            let parity_idx = shard_index - data_shards as usize;
            parity.into_iter().nth(parity_idx).ok_or_else(|| {
                crate::error::DataPlaneError::ChunkEngineError(format!(
                    "RS encode did not produce parity shard {parity_idx}"
                ))
            })?
        };

        let header = ChunkHeader {
            magic: *b"NVAC",
            version: 1,
            flags: 0,
            checksum: crc32c::crc32c(&reconstructed_shard),
            data_len: reconstructed_shard.len() as u32,
            _reserved: [0; 2],
        };
        let mut prepared = Vec::with_capacity(ChunkHeader::SIZE + reconstructed_shard.len());
        prepared.extend_from_slice(&header.to_bytes());
        prepared.extend_from_slice(&reconstructed_shard);

        let shard_id = format!("{chunk_id}:shard:{shard_index}");
        self.local_store.put(&shard_id, &prepared).await?;
        Ok(())
    }

    /// Remove a chunk replica from a node. Single-node: deletes from the
    /// local store.
    pub async fn remove_replica(&self, chunk_id: &str, node_id: &str) -> Result<()> {
        self.ensure_local(node_id)?;
        self.local_store.delete(chunk_id).await?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::backend::chunk_store::{
        ChunkHeader, ChunkStore as ChunkStoreTrait, ChunkStoreStats,
    };
    use async_trait::async_trait;
    use std::collections::HashMap;
    use std::sync::Mutex;

    const LOCAL_NODE: &str = "local-node-1";

    /// In-memory ChunkStore used by these tests so the operations layer
    /// can be exercised without SPDK or a filesystem-backed backend.
    struct MemChunkStore {
        inner: Mutex<HashMap<String, Vec<u8>>>,
    }

    impl MemChunkStore {
        fn new() -> Self {
            Self {
                inner: Mutex::new(HashMap::new()),
            }
        }
    }

    #[async_trait]
    impl ChunkStoreTrait for MemChunkStore {
        async fn put(&self, chunk_id: &str, data: &[u8]) -> Result<()> {
            self.inner
                .lock()
                .unwrap()
                .insert(chunk_id.to_string(), data.to_vec());
            Ok(())
        }
        async fn get(&self, chunk_id: &str) -> Result<Vec<u8>> {
            self.inner
                .lock()
                .unwrap()
                .get(chunk_id)
                .cloned()
                .ok_or_else(|| {
                    crate::error::DataPlaneError::ChunkEngineError(format!(
                        "chunk {chunk_id} not found"
                    ))
                })
        }
        async fn delete(&self, chunk_id: &str) -> Result<()> {
            self.inner.lock().unwrap().remove(chunk_id).ok_or_else(|| {
                crate::error::DataPlaneError::ChunkEngineError(format!(
                    "chunk {chunk_id} not found"
                ))
            })?;
            Ok(())
        }
        async fn exists(&self, chunk_id: &str) -> Result<bool> {
            Ok(self.inner.lock().unwrap().contains_key(chunk_id))
        }
        async fn stats(&self) -> Result<ChunkStoreStats> {
            Ok(ChunkStoreStats {
                backend_name: "mem".into(),
                total_bytes: 0,
                used_bytes: 0,
                data_bytes: 0,
                chunk_count: self.inner.lock().unwrap().len() as u64,
            })
        }
    }

    fn make_chunk_data(data: &[u8]) -> Vec<u8> {
        let header = ChunkHeader {
            magic: *b"NVAC",
            version: 1,
            flags: 0,
            checksum: crc32c::crc32c(data),
            data_len: data.len() as u32,
            _reserved: [0; 2],
        };
        let mut buf = Vec::with_capacity(ChunkHeader::SIZE + data.len());
        buf.extend_from_slice(&header.to_bytes());
        buf.extend_from_slice(data);
        buf
    }

    fn fake_chunk_id() -> String {
        "aabbccdd00112233aabbccdd00112233aabbccdd00112233aabbccdd00112233".to_string()
    }

    async fn make_ops() -> ChunkOperations {
        let store = MemChunkStore::new();
        ChunkOperations::new(LOCAL_NODE.to_string(), Arc::new(store))
    }

    #[tokio::test]
    async fn replicate_local_to_local() {
        let ops = make_ops().await;
        let chunk_id = fake_chunk_id();
        let data = make_chunk_data(b"hello replication");

        ops.local_store.put(&chunk_id, &data).await.unwrap();
        ops.replicate_chunk(&chunk_id, LOCAL_NODE, LOCAL_NODE)
            .await
            .unwrap();

        let got = ops.local_store.get(&chunk_id).await.unwrap();
        assert_eq!(got, data);
    }

    #[tokio::test]
    async fn replicate_preserves_data() {
        let ops = make_ops().await;
        let chunk_id = fake_chunk_id();
        let payload = b"exact data must survive replication";
        let data = make_chunk_data(payload);

        ops.local_store.put(&chunk_id, &data).await.unwrap();
        ops.replicate_chunk(&chunk_id, LOCAL_NODE, LOCAL_NODE)
            .await
            .unwrap();

        let got = ops.local_store.get(&chunk_id).await.unwrap();
        assert_eq!(got, data);
    }

    #[tokio::test]
    async fn remove_local_replica() {
        let ops = make_ops().await;
        let chunk_id = fake_chunk_id();
        let data = make_chunk_data(b"will be removed");

        ops.local_store.put(&chunk_id, &data).await.unwrap();
        assert!(ops.local_store.exists(&chunk_id).await.unwrap());

        ops.remove_replica(&chunk_id, LOCAL_NODE).await.unwrap();
        assert!(!ops.local_store.exists(&chunk_id).await.unwrap());
    }

    #[tokio::test]
    async fn remove_nonexistent_chunk_returns_error() {
        let ops = make_ops().await;
        let chunk_id = fake_chunk_id();
        let result = ops.remove_replica(&chunk_id, LOCAL_NODE).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn replicate_nonexistent_source_returns_error() {
        let ops = make_ops().await;
        let chunk_id = fake_chunk_id();
        let result = ops.replicate_chunk(&chunk_id, LOCAL_NODE, LOCAL_NODE).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn non_local_target_is_rejected() {
        let ops = make_ops().await;
        let chunk_id = fake_chunk_id();
        let data = make_chunk_data(b"data");
        ops.local_store.put(&chunk_id, &data).await.unwrap();

        let err = ops
            .replicate_chunk(&chunk_id, LOCAL_NODE, "other-host")
            .await
            .unwrap_err();
        assert!(format!("{err}").contains("non-local"));
    }
}
