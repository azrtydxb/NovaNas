//! tonic gRPC client for `MetaService`, dialled over a Unix-domain socket.
//!
//! Meta is a single-host daemon and listens on `/var/run/novanas/meta.sock`
//! by default. tonic supports UDS via a custom `Channel` connector that
//! ignores the URI authority and dials the configured socket path.
//!
//! The wire contract is the canonical wrapper-style `MetaService` defined
//! in `storage/api/proto/meta/meta.proto`. Wrapper-style RPCs key on UUIDs
//! and return full `ChunkMap`s; the frontend's `MetaClient` trait keeps
//! its name-keyed, range-sliced shape so the rest of the daemon (the
//! reconciler, the chunk_map cache) can stay unchanged. Translation from
//! `name → uuid` is done client-side via `ListVolumes`; range slicing is
//! done in-memory after a `GetChunkMap`.

use std::path::PathBuf;
use std::sync::Arc;

use async_trait::async_trait;
use hyper_util::rt::TokioIo;
use tokio::net::UnixStream;
use tonic::transport::{Channel, Endpoint, Uri};
use tower::service_fn;

use crate::error::{FrontendError, Result};
use crate::proto::meta::{
    meta_service_client::MetaServiceClient, ChunkMap, ChunkPlacement, GetChunkMapRequest,
    HeartbeatRequest, HeartbeatResponse, ListVolumesRequest, ListVolumesResponse, Volume,
};

/// Trait abstracting the meta client for testing.
#[async_trait]
pub trait MetaClient: Send + Sync {
    /// Look up a volume by user-visible name. The wrapper-style RPC
    /// `GetVolume` keys on UUID; we list and filter to preserve the
    /// frontend's name-based ergonomics.
    async fn get_volume(&self, name: &str) -> Result<Volume>;
    /// List every volume known to meta.
    async fn list_volumes(&self) -> Result<ListVolumesResponse>;
    /// Return the chunk-map placements for the half-open index range
    /// `[chunk_index_lo, chunk_index_hi)`. The wrapper proto returns the
    /// full `ChunkMap` per volume, so we slice client-side.
    async fn get_chunk_map(
        &self,
        volume_name: &str,
        chunk_index_lo: u64,
        chunk_index_hi: u64,
    ) -> Result<Vec<ChunkPlacement>>;
    async fn heartbeat(&self, req: HeartbeatRequest) -> Result<HeartbeatResponse>;
}

/// Production tonic client wired over a Unix-domain socket.
pub struct UdsMetaClient {
    inner: tokio::sync::Mutex<MetaServiceClient<Channel>>,
}

impl UdsMetaClient {
    /// Build a UDS-backed channel. The URI authority is a placeholder —
    /// the actual dial happens inside `service_fn`.
    pub async fn connect(socket_path: impl Into<PathBuf>) -> Result<Self> {
        let path: Arc<PathBuf> = Arc::new(socket_path.into());
        let connector_path = path.clone();

        let channel = Endpoint::try_from("http://[::1]:50051")
            .map_err(|e| FrontendError::Meta(format!("endpoint: {}", e)))?
            .connect_with_connector(service_fn(move |_uri: Uri| {
                let p = connector_path.clone();
                async move {
                    let stream = UnixStream::connect(p.as_path()).await?;
                    Ok::<_, std::io::Error>(TokioIo::new(stream))
                }
            }))
            .await
            .map_err(|e| FrontendError::Meta(format!("connect {:?}: {}", path, e)))?;
        Ok(Self {
            inner: tokio::sync::Mutex::new(MetaServiceClient::new(channel)),
        })
    }

    /// Construct from an existing tonic Channel — useful for tests that
    /// stand up an in-process tonic server.
    pub fn from_channel(channel: Channel) -> Self {
        Self {
            inner: tokio::sync::Mutex::new(MetaServiceClient::new(channel)),
        }
    }

    async fn list_all(&self) -> Result<Vec<Volume>> {
        let resp = self
            .inner
            .lock()
            .await
            .list_volumes(ListVolumesRequest {
                pool_uuid: String::new(),
            })
            .await?;
        Ok(resp.into_inner().volumes)
    }

    async fn fetch_chunk_map(&self, volume_uuid: &str) -> Result<ChunkMap> {
        let resp = self
            .inner
            .lock()
            .await
            .get_chunk_map(GetChunkMapRequest {
                volume_uuid: volume_uuid.to_string(),
            })
            .await?;
        let inner = resp.into_inner();
        Ok(inner.chunk_map.unwrap_or(ChunkMap {
            volume_uuid: volume_uuid.to_string(),
            chunks: Vec::new(),
        }))
    }
}

#[async_trait]
impl MetaClient for UdsMetaClient {
    async fn get_volume(&self, name: &str) -> Result<Volume> {
        let volumes = self.list_all().await?;
        volumes
            .into_iter()
            .find(|v| v.name == name)
            .ok_or_else(|| FrontendError::Meta(format!("volume {} not found", name)))
    }

    async fn list_volumes(&self) -> Result<ListVolumesResponse> {
        let volumes = self.list_all().await?;
        Ok(ListVolumesResponse { volumes })
    }

    async fn get_chunk_map(
        &self,
        volume_name: &str,
        chunk_index_lo: u64,
        chunk_index_hi: u64,
    ) -> Result<Vec<ChunkPlacement>> {
        let volume = self.get_volume(volume_name).await?;
        let map = self.fetch_chunk_map(&volume.uuid).await?;
        Ok(map
            .chunks
            .into_iter()
            .filter(|p| {
                let i = p.index as u64;
                i >= chunk_index_lo && i < chunk_index_hi
            })
            .collect())
    }

    async fn heartbeat(&self, req: HeartbeatRequest) -> Result<HeartbeatResponse> {
        let resp = self.inner.lock().await.heartbeat(req).await?;
        Ok(resp.into_inner())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn endpoint_construction_does_not_dial() {
        // Constructing an Endpoint should not require a server to exist.
        // This is a smoke test for the URL parsing only.
        let _ = Endpoint::try_from("http://[::1]:50051").unwrap();
    }

    #[test]
    fn proto_types_compile_in_test_scope() {
        let _placement = ChunkPlacement {
            index: 0,
            chunk_id: "abc".into(),
            disk_uuids: vec!["disk-x".into()],
        };
        let _hb = HeartbeatRequest {
            client_id: "frontend".into(),
        };
    }
}
