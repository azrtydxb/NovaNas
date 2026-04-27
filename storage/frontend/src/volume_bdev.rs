//! Volume bdev — the SPDK custom block device that fronts each user volume.
//!
//! On a real SPDK-linked build (the `spdk-sys` feature) this module
//! registers a custom bdev module whose `read`/`write`/`flush` callbacks
//! delegate to the frontend's `ChunkEngine`. That implementation is in
//! `spdk_volume_bdev.rs`, behind the feature gate.
//!
//! The trait below describes the surface the rest of the crate uses
//! and is always present so api_subscriber + tests can compile without
//! SPDK.

use std::sync::Arc;

use async_trait::async_trait;

use crate::chunk_engine::ChunkEngine;
use crate::error::Result;

/// One registered volume bdev.
#[derive(Debug, Clone)]
pub struct VolumeBdevHandle {
    pub volume_name: String,
    pub bdev_name: String,
    pub size_bytes: u64,
    pub block_size: u32,
}

impl VolumeBdevHandle {
    pub fn bdev_name_for(volume_name: &str) -> String {
        format!("novanas_{}", volume_name)
    }
}

#[async_trait]
pub trait VolumeBdevManager: Send + Sync {
    async fn create(
        &self,
        volume_name: &str,
        size_bytes: u64,
        block_size: u32,
    ) -> Result<VolumeBdevHandle>;
    async fn destroy(&self, volume_name: &str) -> Result<()>;
    async fn list(&self) -> Result<Vec<VolumeBdevHandle>>;
}

/// In-process bdev manager double for tests and SPDK-less builds. Keeps
/// references to the ChunkEngine (so it could be invoked from a fake
/// I/O harness if needed) but does not actually expose anything to the
/// kernel.
pub struct NoopVolumeBdevManager {
    engine: Arc<ChunkEngine>,
    state: tokio::sync::Mutex<std::collections::HashMap<String, VolumeBdevHandle>>,
}

impl NoopVolumeBdevManager {
    pub fn new(engine: Arc<ChunkEngine>) -> Self {
        Self {
            engine,
            state: tokio::sync::Mutex::new(Default::default()),
        }
    }

    /// Engine reference for I/O routing in test harnesses.
    pub fn engine(&self) -> &Arc<ChunkEngine> {
        &self.engine
    }
}

#[async_trait]
impl VolumeBdevManager for NoopVolumeBdevManager {
    async fn create(
        &self,
        volume_name: &str,
        size_bytes: u64,
        block_size: u32,
    ) -> Result<VolumeBdevHandle> {
        let h = VolumeBdevHandle {
            volume_name: volume_name.to_string(),
            bdev_name: VolumeBdevHandle::bdev_name_for(volume_name),
            size_bytes,
            block_size,
        };
        self.state
            .lock()
            .await
            .insert(volume_name.to_string(), h.clone());
        Ok(h)
    }

    async fn destroy(&self, volume_name: &str) -> Result<()> {
        self.state.lock().await.remove(volume_name);
        Ok(())
    }

    async fn list(&self) -> Result<Vec<VolumeBdevHandle>> {
        Ok(self.state.lock().await.values().cloned().collect())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::chunk_map_cache::ChunkMapCache;
    use crate::ndp_client::tests::FakeNdp;
    use crate::ndp_client::NdpChunkClient;

    fn make_engine() -> Arc<ChunkEngine> {
        let cache = Arc::new(ChunkMapCache::in_memory());
        let ndp = Arc::new(FakeNdp::new()) as Arc<dyn NdpChunkClient>;
        Arc::new(ChunkEngine::new(cache, ndp))
    }

    #[test]
    fn bdev_name_format_is_stable() {
        assert_eq!(
            VolumeBdevHandle::bdev_name_for("foo"),
            "novanas_foo".to_string()
        );
    }

    #[tokio::test]
    async fn noop_manager_lifecycle() {
        let mgr = NoopVolumeBdevManager::new(make_engine());
        let h = mgr.create("v1", 4096 * 256, 4096).await.unwrap();
        assert_eq!(h.bdev_name, "novanas_v1");
        let list = mgr.list().await.unwrap();
        assert_eq!(list.len(), 1);
        mgr.destroy("v1").await.unwrap();
        assert!(mgr.list().await.unwrap().is_empty());
    }
}
