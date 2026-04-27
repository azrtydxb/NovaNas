//! Integration test: drive the ChunkEngine end-to-end against the
//! `FakeNdp` test double + a fake meta. Verifies split / assemble,
//! cross-chunk reads, partial-block edge handling, and write-cache
//! absorption.

use std::collections::HashMap;
use std::sync::Arc;
use std::sync::Mutex as StdMutex;

use async_trait::async_trait;

use novanas_frontend::chunk_engine::{ChunkEngine, CHUNK_SIZE};
use novanas_frontend::chunk_map_cache::ChunkMapCache;
use novanas_frontend::error::{FrontendError, Result};
use novanas_frontend::ndp_client::NdpChunkClient;
use novanas_frontend::write_cache::SUB_BLOCK_SIZE;

#[derive(Default)]
struct FakeNdp {
    store: StdMutex<HashMap<String, Vec<u8>>>,
}
#[async_trait]
impl NdpChunkClient for FakeNdp {
    async fn put_chunk(&self, chunk_id: &str, payload: &[u8]) -> Result<()> {
        self.store
            .lock()
            .unwrap()
            .insert(chunk_id.to_string(), payload.to_vec());
        Ok(())
    }
    async fn get_chunk(&self, chunk_id: &str) -> Result<Vec<u8>> {
        self.store
            .lock()
            .unwrap()
            .get(chunk_id)
            .cloned()
            .ok_or_else(|| FrontendError::Ndp(format!("missing {}", chunk_id)))
    }
    async fn delete_chunk(&self, chunk_id: &str) -> Result<()> {
        self.store.lock().unwrap().remove(chunk_id);
        Ok(())
    }
}

fn engine() -> (Arc<FakeNdp>, ChunkEngine) {
    let ndp = Arc::new(FakeNdp::default());
    let cache = Arc::new(ChunkMapCache::in_memory());
    let e = ChunkEngine::new(cache, ndp.clone() as Arc<dyn NdpChunkClient>);
    (ndp, e)
}

#[tokio::test]
async fn write_assemble_read_full_volume() {
    let (_ndp, engine) = engine();
    // Two full chunks + a partial.
    let total = 2 * CHUNK_SIZE + 17;
    let mut data = vec![0u8; total];
    for (i, b) in data.iter_mut().enumerate() {
        *b = (i % 251) as u8;
    }
    engine.write_volume("vol", 0, &data).await.unwrap();
    let got = engine.read_volume("vol", 0, total as u64).await.unwrap();
    assert_eq!(got, data);
}

#[tokio::test]
async fn read_partial_at_volume_edge() {
    let (_ndp, engine) = engine();
    let mut data = vec![0u8; 2 * CHUNK_SIZE];
    for (i, b) in data.iter_mut().enumerate() {
        *b = (i % 251) as u8;
    }
    engine.write_volume("vol", 0, &data).await.unwrap();
    // Read crossing the chunk boundary.
    let lo = (CHUNK_SIZE as u64) - 8;
    let got = engine.read_volume("vol", lo, 16).await.unwrap();
    assert_eq!(got, data[lo as usize..lo as usize + 16]);
}

#[tokio::test]
async fn write_cache_absorbs_aligned_subblocks() {
    let (_ndp, engine) = engine();
    let sb = vec![0xCDu8; SUB_BLOCK_SIZE as usize];
    engine.sub_block_write("vol", 0, &sb).await.unwrap();
    // The cache must serve a subsequent read without hitting NDP at all
    // (FakeNdp would 404 since we never wrote a chunk).
    let got = engine.sub_block_read("vol", 0, 4096).await.unwrap();
    assert_eq!(got, vec![0xCDu8; 4096]);
}

#[tokio::test]
async fn flush_persists_cached_writes() {
    let (ndp, engine) = engine();
    let sb = vec![0xEEu8; SUB_BLOCK_SIZE as usize];
    engine.sub_block_write("vol", 0, &sb).await.unwrap();
    engine.flush("vol").await.unwrap();
    // After flush at least one chunk must exist in NDP.
    assert!(!ndp.store.lock().unwrap().is_empty());
    // Cache is empty post-flush; reads now go through NDP and the
    // recorded chunk_id mapping in the cache.
    let got = engine.read_volume("vol", 0, SUB_BLOCK_SIZE).await.unwrap();
    assert_eq!(got.len(), SUB_BLOCK_SIZE as usize);
    assert_eq!(got[0], 0xEE);
}

#[tokio::test]
async fn unaligned_write_invalidates_overlap() {
    let (_ndp, engine) = engine();
    // First put a clean aligned sub-block in the cache.
    let sb = vec![0x11u8; SUB_BLOCK_SIZE as usize];
    engine.sub_block_write("vol", 0, &sb).await.unwrap();
    // Now write a small unaligned slice into the same range — it must
    // bypass the cache and invalidate the overlapping entry.
    let small = vec![0x22u8; 1024];
    engine.sub_block_write("vol", 100, &small).await.unwrap();
    // Subsequent read must reflect the new bytes (passed through to
    // NDP via flush_single_write) rather than the stale cache.
    let got = engine.sub_block_read("vol", 100, 1024).await.unwrap();
    assert_eq!(got, vec![0x22u8; 1024]);
}
