//! Tonic client for the `MetaService` defined in
//! `storage/api/proto/meta/meta.proto`.
//!
//! Connects to `novanas-meta` over a Unix-domain socket (default
//! `/var/run/novanas/meta.sock`) and exposes thin wrappers around the
//! RPCs the data daemon needs:
//!
//! - [`MetaClient::heartbeat`] — periodic 30s liveness + disk-status push.
//! - [`MetaClient::poll_tasks`] — long-poll for the work queue.
//! - [`MetaClient::ack_task`] — final disposition for a task.
//! - [`MetaClient::claim_disk`] — explicit ClaimDisk RPC (used during
//!   superblock-driven onboarding flows; the task queue covers the
//!   common path).
//!
//! Connection establishment is lazy via tonic's `Endpoint` so callers can
//! construct a `MetaClient` even when the meta socket has not been
//! created yet (the connect attempt happens on the first RPC).

use std::path::{Path, PathBuf};
use std::time::Duration;

use tokio::net::UnixStream;
use tonic::transport::{Channel, Endpoint, Uri};
use tower::service_fn;

use crate::error::{DataPlaneError, Result};
use crate::transport::meta_proto::meta_service_client::MetaServiceClient;
use crate::transport::meta_proto::{
    AckTaskRequest, ClaimDiskRequest, DaemonKind, Disk, DiskRef, HeartbeatRequest,
    HeartbeatResponse, PollTasksRequest, TaskBatch,
};

/// Default UDS path the meta daemon listens on.
pub const DEFAULT_META_SOCKET: &str = "/var/run/novanas/meta.sock";

/// Configuration for the meta client.
#[derive(Debug, Clone)]
pub struct MetaClientConfig {
    /// Path to the meta daemon's UDS.
    pub socket_path: PathBuf,
    /// Connect timeout per attempt.
    pub connect_timeout: Duration,
    /// Per-RPC default deadline.
    pub rpc_timeout: Duration,
}

impl Default for MetaClientConfig {
    fn default() -> Self {
        Self {
            socket_path: PathBuf::from(DEFAULT_META_SOCKET),
            connect_timeout: Duration::from_secs(5),
            rpc_timeout: Duration::from_secs(30),
        }
    }
}

/// Tonic-backed client that speaks `MetaService` over a UDS.
pub struct MetaClient {
    inner: MetaServiceClient<Channel>,
    config: MetaClientConfig,
}

impl MetaClient {
    /// Connect to the meta daemon at `config.socket_path`.
    ///
    /// The tonic `Endpoint` is configured with a fake URI (`http://meta`)
    /// — the actual transport is the UDS connector below.
    pub async fn connect(config: MetaClientConfig) -> Result<Self> {
        let socket_path = config.socket_path.clone();
        let endpoint = Endpoint::try_from("http://meta")
            .map_err(|e| DataPlaneError::TransportError(format!("meta endpoint invalid: {e}")))?
            .connect_timeout(config.connect_timeout)
            .timeout(config.rpc_timeout);

        let channel = endpoint
            .connect_with_connector(service_fn(move |_: Uri| {
                let path = socket_path.clone();
                async move {
                    UnixStream::connect(&path)
                        .await
                        .map(hyper_util::rt::TokioIo::new)
                }
            }))
            .await
            .map_err(|e| {
                DataPlaneError::TransportError(format!(
                    "meta UDS connect {}: {e}",
                    config.socket_path.display()
                ))
            })?;

        Ok(Self {
            inner: MetaServiceClient::new(channel),
            config,
        })
    }

    /// Path the client is bound to.
    pub fn socket_path(&self) -> &Path {
        &self.config.socket_path
    }

    /// Send a heartbeat. The daemon pushes its disk-status records and
    /// receives the desired CRUSH digest + a hint about the queue depth.
    pub async fn heartbeat(
        &mut self,
        node_id: &str,
        version: &str,
        disks: Vec<Disk>,
    ) -> Result<HeartbeatResponse> {
        let req = HeartbeatRequest {
            node_id: node_id.to_string(),
            kind: DaemonKind::Data as i32,
            version: version.to_string(),
            disks,
        };
        let resp = self
            .inner
            .heartbeat(req)
            .await
            .map_err(|s| DataPlaneError::TransportError(format!("heartbeat: {s}")))?;
        Ok(resp.into_inner())
    }

    /// Long-poll the meta task queue. Returns whatever batch the meta side
    /// has ready before `deadline_ms` expires (possibly empty).
    pub async fn poll_tasks(
        &mut self,
        node_id: &str,
        max_tasks: u32,
        deadline_ms: u32,
    ) -> Result<TaskBatch> {
        let req = PollTasksRequest {
            node_id: node_id.to_string(),
            max_tasks,
            deadline_ms,
        };
        let resp = self
            .inner
            .poll_tasks(req)
            .await
            .map_err(|s| DataPlaneError::TransportError(format!("poll_tasks: {s}")))?;
        Ok(resp.into_inner())
    }

    /// Ack a task — `success=true` removes it, `success=false` requeues it
    /// with `error_message` recorded for operators.
    pub async fn ack_task(
        &mut self,
        task_id: &str,
        success: bool,
        error_message: &str,
    ) -> Result<()> {
        let req = AckTaskRequest {
            task_id: task_id.to_string(),
            success,
            error_message: error_message.to_string(),
        };
        self.inner
            .ack_task(req)
            .await
            .map_err(|s| DataPlaneError::TransportError(format!("ack_task: {s}")))?;
        Ok(())
    }

    /// Issue an explicit `ClaimDisk` RPC. The data daemon mostly receives
    /// claim work via [`Self::poll_tasks`] (`TASK_KIND_CLAIM_DISK`), but
    /// the explicit form is useful for re-binding a disk after restart.
    pub async fn claim_disk(
        &mut self,
        wwn: &str,
        pool_name: &str,
        role: i32,
        force: bool,
    ) -> Result<Disk> {
        let req = ClaimDiskRequest {
            wwn: wwn.to_string(),
            pool_name: pool_name.to_string(),
            role,
            force,
        };
        let resp = self
            .inner
            .claim_disk(req)
            .await
            .map_err(|s| DataPlaneError::TransportError(format!("claim_disk: {s}")))?;
        Ok(resp.into_inner())
    }

    /// Mirror of `MetaService::DeleteDisk` for clean release.
    pub async fn delete_disk(&mut self, wwn: &str) -> Result<()> {
        let req = DiskRef {
            wwn: wwn.to_string(),
        };
        self.inner
            .delete_disk(req)
            .await
            .map_err(|s| DataPlaneError::TransportError(format!("delete_disk: {s}")))?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_config_uses_meta_socket() {
        let cfg = MetaClientConfig::default();
        assert_eq!(cfg.socket_path, PathBuf::from(DEFAULT_META_SOCKET));
    }

    #[tokio::test]
    async fn connect_to_missing_socket_returns_transport_error() {
        let dir = tempfile::tempdir().unwrap();
        let cfg = MetaClientConfig {
            socket_path: dir.path().join("does-not-exist.sock"),
            connect_timeout: Duration::from_millis(50),
            rpc_timeout: Duration::from_secs(1),
        };
        match MetaClient::connect(cfg).await {
            Ok(_) => panic!("expected connect to a missing socket to fail"),
            Err(DataPlaneError::TransportError(_)) => {}
            Err(other) => panic!("expected TransportError, got {other:?}"),
        }
    }
}
