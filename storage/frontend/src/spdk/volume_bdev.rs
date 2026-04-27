//! SPDK-linked implementation of the volume bdev (placeholder port).
//!
//! See spdk/env.rs note: when `spdk-sys` is on, this would register a
//! custom bdev module whose I/O callbacks fan out to the frontend's
//! `ChunkEngine`. The full implementation port from
//! `dataplane/src/bdev/novanas_bdev.rs` is deferred to Agent C's
//! consolidation pass; the placeholder here keeps the build green.

use std::sync::Arc;

use async_trait::async_trait;

use crate::chunk_engine::ChunkEngine;
use crate::error::{FrontendError, Result};
use crate::volume_bdev::{VolumeBdevHandle, VolumeBdevManager};

pub struct SpdkVolumeBdevManager {
    engine: Arc<ChunkEngine>,
    state: tokio::sync::Mutex<std::collections::HashMap<String, VolumeBdevHandle>>,
}

impl SpdkVolumeBdevManager {
    pub fn new(engine: Arc<ChunkEngine>) -> Self {
        Self {
            engine,
            state: tokio::sync::Mutex::new(Default::default()),
        }
    }

    pub fn engine(&self) -> &Arc<ChunkEngine> {
        &self.engine
    }
}

#[async_trait]
impl VolumeBdevManager for SpdkVolumeBdevManager {
    async fn create(
        &self,
        volume_name: &str,
        size_bytes: u64,
        block_size: u32,
    ) -> Result<VolumeBdevHandle> {
        // Placeholder until Agent C ports novanas_bdev.rs across.
        log::warn!(
            "SpdkVolumeBdevManager::create({}, {}, {}): SPDK port pending; \
             returning a logical handle without actually registering an SPDK bdev",
            volume_name,
            size_bytes,
            block_size
        );
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
        if self.state.lock().await.remove(volume_name).is_none() {
            return Err(FrontendError::Bdev(format!(
                "bdev {} not found",
                volume_name
            )));
        }
        Ok(())
    }

    async fn list(&self) -> Result<Vec<VolumeBdevHandle>> {
        Ok(self.state.lock().await.values().cloned().collect())
    }
}
