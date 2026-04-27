//! tonic gRPC client for `MetaService`, dialled over a Unix-domain socket.
//!
//! Meta is a single-host daemon and listens on `/var/run/novanas/meta.sock`
//! by default. tonic supports UDS via a custom `Channel` connector that
//! ignores the URI authority and dials the configured socket path.

use std::path::PathBuf;
use std::sync::Arc;

use async_trait::async_trait;
use hyper_util::rt::TokioIo;
use tokio::net::UnixStream;
use tonic::transport::{Channel, Endpoint, Uri};
use tower::service_fn;

use crate::error::{FrontendError, Result};
use crate::proto::meta::{
    meta_service_client::MetaServiceClient, ChunkMapSlice, GetChunkMapRequest, HeartbeatRequest,
    HeartbeatResponse, Volume, VolumeList, VolumeRef,
};

/// Trait abstracting the meta client for testing.
#[async_trait]
pub trait MetaClient: Send + Sync {
    async fn get_volume(&self, name: &str) -> Result<Volume>;
    async fn list_volumes(&self) -> Result<VolumeList>;
    async fn get_chunk_map(
        &self,
        volume_name: &str,
        chunk_index_lo: u64,
        chunk_index_hi: u64,
    ) -> Result<ChunkMapSlice>;
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
}

#[async_trait]
impl MetaClient for UdsMetaClient {
    async fn get_volume(&self, name: &str) -> Result<Volume> {
        let resp = self
            .inner
            .lock()
            .await
            .get_volume(VolumeRef {
                name: name.to_string(),
            })
            .await?;
        Ok(resp.into_inner())
    }

    async fn list_volumes(&self) -> Result<VolumeList> {
        let resp = self.inner.lock().await.list_volumes(()).await?;
        Ok(resp.into_inner())
    }

    async fn get_chunk_map(
        &self,
        volume_name: &str,
        chunk_index_lo: u64,
        chunk_index_hi: u64,
    ) -> Result<ChunkMapSlice> {
        let resp = self
            .inner
            .lock()
            .await
            .get_chunk_map(GetChunkMapRequest {
                volume_name: volume_name.to_string(),
                chunk_index_lo,
                chunk_index_hi,
            })
            .await?;
        Ok(resp.into_inner())
    }

    async fn heartbeat(&self, req: HeartbeatRequest) -> Result<HeartbeatResponse> {
        let resp = self.inner.lock().await.heartbeat(req).await?;
        Ok(resp.into_inner())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::meta::{ChunkMapEntry, DaemonKind};

    #[tokio::test]
    async fn endpoint_construction_does_not_dial() {
        // Constructing an Endpoint should not require a server to exist.
        // This is a smoke test for the URL parsing only.
        let _ = Endpoint::try_from("http://[::1]:50051").unwrap();
    }

    #[test]
    fn proto_types_compile_in_test_scope() {
        let _entry = ChunkMapEntry {
            chunk_index: 0,
            chunk_id: "abc".into(),
            disk_wwns: vec!["wwn-x".into()],
        };
        let _kind = DaemonKind::Frontend;
    }
}
