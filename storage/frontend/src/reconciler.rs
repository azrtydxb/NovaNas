//! Glue between the API subscriber, the meta client, the volume bdev
//! manager, and the NVMe-oF target. A new `BlockVolume` from the API
//! triggers:
//!
//!   1. `meta.GetVolume(name)` → authoritative size, chunk_size, phase.
//!   2. `bdev_mgr.create(...)` → register a volume bdev.
//!   3. `nvmf.add_subsystem(...)` → expose the bdev as NVMe-oF.
//!
//! Removal is the reverse path. Errors are logged and surfaced; the
//! subscriber loop will retry on the next tick.

use std::sync::Arc;

use async_trait::async_trait;

use crate::api_subscriber::{ApiBlockVolume, BlockVolumeReconciler};
use crate::error::{FrontendError, Result};
use crate::meta_client::MetaClient;
use crate::nvmf::{NvmfTarget, SubsystemSpec};
use crate::volume_bdev::{VolumeBdevHandle, VolumeBdevManager};

/// Default block size exposed to NVMe-oF initiators. 4 KiB matches the
/// typical filesystem alignment.
pub const DEFAULT_BLOCK_SIZE: u32 = 4096;

pub struct VolumeReconciler {
    meta: Arc<dyn MetaClient>,
    bdev_mgr: Arc<dyn VolumeBdevManager>,
    nvmf: Arc<dyn NvmfTarget>,
    listen_address: String,
    listen_port: u16,
}

impl VolumeReconciler {
    pub fn new(
        meta: Arc<dyn MetaClient>,
        bdev_mgr: Arc<dyn VolumeBdevManager>,
        nvmf: Arc<dyn NvmfTarget>,
        listen_address: impl Into<String>,
        listen_port: u16,
    ) -> Self {
        Self {
            meta,
            bdev_mgr,
            nvmf,
            listen_address: listen_address.into(),
            listen_port,
        }
    }

    async fn provision(&self, vol: &ApiBlockVolume) -> Result<VolumeBdevHandle> {
        // Pull authoritative metadata from meta. If meta is the source
        // of truth on size_bytes (it is — meta runs CreateVolume), we
        // prefer its number even if the API JSON has its own.
        let size_bytes = match self.meta.get_volume(&vol.name).await {
            Ok(v) => {
                if v.size_bytes > 0 {
                    v.size_bytes
                } else {
                    vol.size_bytes
                }
            }
            Err(e) => {
                log::warn!(
                    "reconciler: meta.GetVolume({}) failed: {} — using API size {}",
                    vol.name,
                    e,
                    vol.size_bytes
                );
                vol.size_bytes
            }
        };
        if size_bytes == 0 {
            return Err(FrontendError::Config(format!(
                "BlockVolume {} has zero size_bytes from both meta and API",
                vol.name
            )));
        }
        let handle = self
            .bdev_mgr
            .create(&vol.name, size_bytes, DEFAULT_BLOCK_SIZE)
            .await?;
        let spec = SubsystemSpec {
            volume_name: vol.name.clone(),
            size_bytes,
            bdev_name: handle.bdev_name.clone(),
            listen_address: self.listen_address.clone(),
            listen_port: self.listen_port,
        };
        self.nvmf.add_subsystem(&spec).await?;
        Ok(handle)
    }

    async fn deprovision(&self, name: &str) -> Result<()> {
        // Best-effort teardown: continue past errors so we don't leak
        // half-removed state. The first error is returned.
        let nvmf_err = self.nvmf.remove_subsystem(name).await.err();
        let bdev_err = self.bdev_mgr.destroy(name).await.err();
        if let Some(e) = nvmf_err {
            return Err(e);
        }
        if let Some(e) = bdev_err {
            return Err(e);
        }
        Ok(())
    }
}

#[async_trait]
impl BlockVolumeReconciler for VolumeReconciler {
    async fn on_volume_added(&self, vol: &ApiBlockVolume) -> Result<()> {
        log::info!(
            "BlockVolume added: {} (pool={}, size_bytes={})",
            vol.name,
            vol.pool,
            vol.size_bytes
        );
        let _ = self.provision(vol).await?;
        Ok(())
    }

    async fn on_volume_removed(&self, name: &str) -> Result<()> {
        log::info!("BlockVolume removed: {}", name);
        self.deprovision(name).await
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::chunk_engine::ChunkEngine;
    use crate::chunk_map_cache::ChunkMapCache;
    use crate::ndp_client::tests::FakeNdp;
    use crate::ndp_client::NdpChunkClient;
    use crate::nvmf::NoopNvmfTarget;
    use crate::proto::meta::{
        ChunkPlacement, HeartbeatRequest, HeartbeatResponse, ListVolumesResponse, Volume,
    };
    use crate::volume_bdev::NoopVolumeBdevManager;

    struct FixedSizeMeta {
        size: u64,
    }
    #[async_trait]
    impl MetaClient for FixedSizeMeta {
        async fn get_volume(&self, name: &str) -> Result<Volume> {
            Ok(Volume {
                name: name.to_string(),
                uuid: "uuid".into(),
                pool_uuid: "pool".into(),
                size_bytes: self.size,
                chunk_size_bytes: 4 * 1024 * 1024,
                chunk_count: self.size.div_ceil(4 * 1024 * 1024),
                protection: None,
                generation: 0,
            })
        }
        async fn list_volumes(&self) -> Result<ListVolumesResponse> {
            Ok(ListVolumesResponse { volumes: vec![] })
        }
        async fn get_chunk_map(&self, _: &str, _: u64, _: u64) -> Result<Vec<ChunkPlacement>> {
            Ok(Vec::new())
        }
        async fn heartbeat(&self, _: HeartbeatRequest) -> Result<HeartbeatResponse> {
            Ok(HeartbeatResponse {
                server_unix_secs: 0,
            })
        }
    }

    fn make_engine() -> Arc<ChunkEngine> {
        let cache = Arc::new(ChunkMapCache::in_memory());
        let ndp = Arc::new(FakeNdp::new()) as Arc<dyn NdpChunkClient>;
        Arc::new(ChunkEngine::new(cache, ndp))
    }

    #[tokio::test]
    async fn add_invokes_bdev_and_nvmf() {
        let meta = Arc::new(FixedSizeMeta { size: 1 << 30 }) as Arc<dyn MetaClient>;
        let bdev = Arc::new(NoopVolumeBdevManager::new(make_engine()));
        let nvmf = Arc::new(NoopNvmfTarget::new());
        let r = VolumeReconciler::new(
            meta,
            bdev.clone() as Arc<dyn VolumeBdevManager>,
            nvmf.clone() as Arc<dyn NvmfTarget>,
            "0.0.0.0",
            4420,
        );
        r.on_volume_added(&ApiBlockVolume {
            name: "v1".into(),
            pool: "p".into(),
            size_bytes: 0,
            phase: "Ready".into(),
        })
        .await
        .unwrap();
        assert_eq!(bdev.list().await.unwrap().len(), 1);
        assert_eq!(
            nvmf.list_subsystems().await.unwrap(),
            vec!["v1".to_string()]
        );
    }

    #[tokio::test]
    async fn remove_tears_down_bdev_and_subsystem() {
        let meta = Arc::new(FixedSizeMeta { size: 1 << 30 }) as Arc<dyn MetaClient>;
        let bdev = Arc::new(NoopVolumeBdevManager::new(make_engine()));
        let nvmf = Arc::new(NoopNvmfTarget::new());
        let r = VolumeReconciler::new(
            meta,
            bdev.clone() as Arc<dyn VolumeBdevManager>,
            nvmf.clone() as Arc<dyn NvmfTarget>,
            "0.0.0.0",
            4420,
        );
        r.on_volume_added(&ApiBlockVolume {
            name: "v1".into(),
            pool: "p".into(),
            size_bytes: 0,
            phase: "Ready".into(),
        })
        .await
        .unwrap();
        r.on_volume_removed("v1").await.unwrap();
        assert!(bdev.list().await.unwrap().is_empty());
        assert!(nvmf.list_subsystems().await.unwrap().is_empty());
    }

    #[tokio::test]
    async fn add_with_zero_size_anywhere_errors() {
        struct ZeroMeta;
        #[async_trait]
        impl MetaClient for ZeroMeta {
            async fn get_volume(&self, name: &str) -> Result<Volume> {
                Ok(Volume {
                    name: name.to_string(),
                    uuid: "u".into(),
                    pool_uuid: "p".into(),
                    size_bytes: 0,
                    chunk_size_bytes: 0,
                    chunk_count: 0,
                    protection: None,
                    generation: 0,
                })
            }
            async fn list_volumes(&self) -> Result<ListVolumesResponse> {
                Ok(ListVolumesResponse { volumes: vec![] })
            }
            async fn get_chunk_map(&self, _: &str, _: u64, _: u64) -> Result<Vec<ChunkPlacement>> {
                Ok(Vec::new())
            }
            async fn heartbeat(&self, _: HeartbeatRequest) -> Result<HeartbeatResponse> {
                Ok(HeartbeatResponse {
                    server_unix_secs: 0,
                })
            }
        }

        let meta = Arc::new(ZeroMeta) as Arc<dyn MetaClient>;
        let bdev = Arc::new(NoopVolumeBdevManager::new(make_engine()));
        let nvmf = Arc::new(NoopNvmfTarget::new());
        let r = VolumeReconciler::new(
            meta,
            bdev as Arc<dyn VolumeBdevManager>,
            nvmf as Arc<dyn NvmfTarget>,
            "0.0.0.0",
            4420,
        );
        let err = r
            .on_volume_added(&ApiBlockVolume {
                name: "v1".into(),
                pool: "p".into(),
                size_bytes: 0,
                phase: "Ready".into(),
            })
            .await
            .unwrap_err();
        assert!(matches!(err, FrontendError::Config(_)));
    }
}
