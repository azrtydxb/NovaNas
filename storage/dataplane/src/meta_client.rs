//! Tonic client for the wrapper-style `MetaService` defined in
//! `storage/api/proto/meta/meta.proto`.
//!
//! Connects to `novanas-meta` over a Unix-domain socket (default
//! `/var/run/novanas/meta.sock`) and exposes thin wrappers around the
//! RPCs the data daemon needs:
//!
//! - [`MetaClient::heartbeat`] — periodic 30s liveness ping. Wrapper-style
//!   `Heartbeat` carries no disk list, so disk presence is pushed
//!   separately via per-disk [`MetaClient::put_disk`] calls (see
//!   [`MetaClient::heartbeat_with_disks`] for the combined helper).
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
    AckTaskRequest, ClaimDiskRequest, DeleteDiskRequest, Disk, HeartbeatRequest, PollTasksRequest,
    PollTasksResponse, PutDiskRequest,
};

/// Default UDS path the meta daemon listens on.
pub const DEFAULT_META_SOCKET: &str = "/var/run/novanas/meta.sock";

/// Result of a heartbeat round-trip. The wrapper-style proto returns just
/// `server_unix_secs`; the dataplane keeps the same return type so call
/// sites stay readable.
#[derive(Debug, Clone)]
pub struct HeartbeatOutcome {
    /// Wall-clock the meta daemon stamped on the response.
    pub server_unix_secs: u64,
    /// Number of disks the daemon successfully pushed via PutDisk.
    pub disks_reported: usize,
}

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

    /// Send a heartbeat ping. The wrapper-style Heartbeat carries only a
    /// client identifier; disk-presence is communicated separately via
    /// `PutDisk`. Returns the meta daemon's wall-clock response.
    pub async fn heartbeat(&mut self, client_id: &str) -> Result<u64> {
        let req = HeartbeatRequest {
            client_id: client_id.to_string(),
        };
        let resp = self
            .inner
            .heartbeat(req)
            .await
            .map_err(|s| DataPlaneError::TransportError(format!("heartbeat: {s}")))?;
        Ok(resp.into_inner().server_unix_secs)
    }

    /// Push or update a single disk record. The data daemon calls this
    /// for every locally-discovered disk on each heartbeat tick.
    pub async fn put_disk(&mut self, disk: Disk) -> Result<Disk> {
        let req = PutDiskRequest { disk: Some(disk) };
        let resp = self
            .inner
            .put_disk(req)
            .await
            .map_err(|s| DataPlaneError::TransportError(format!("put_disk: {s}")))?;
        let inner = resp.into_inner();
        Ok(inner.disk.unwrap_or_default())
    }

    /// Combined heartbeat + disk-presence push. Calls `PutDisk` for every
    /// supplied disk record (errors on individual disks are logged and
    /// counted but not fatal) and finishes with a `Heartbeat` ping.
    pub async fn heartbeat_with_disks(
        &mut self,
        client_id: &str,
        disks: Vec<Disk>,
    ) -> Result<HeartbeatOutcome> {
        let mut reported = 0usize;
        for disk in disks {
            let uuid = disk.uuid.clone();
            match self.put_disk(disk).await {
                Ok(_) => reported += 1,
                Err(e) => {
                    log::warn!("meta_client: PutDisk for {uuid} failed: {e} (will retry next tick)")
                }
            }
        }
        let server_unix_secs = self.heartbeat(client_id).await?;
        Ok(HeartbeatOutcome {
            server_unix_secs,
            disks_reported: reported,
        })
    }

    /// Long-poll the meta task queue. Returns whatever batch the meta side
    /// has ready before the deadline expires (possibly empty).
    pub async fn poll_tasks(&mut self, max_tasks: u32) -> Result<PollTasksResponse> {
        let req = PollTasksRequest { max: max_tasks };
        let resp = self
            .inner
            .poll_tasks(req)
            .await
            .map_err(|s| DataPlaneError::TransportError(format!("poll_tasks: {s}")))?;
        Ok(resp.into_inner())
    }

    /// Ack a task — `success=true` removes it, `success=false` requeues it
    /// with `error` recorded for operators.
    pub async fn ack_task(&mut self, task_id: &str, success: bool, error: &str) -> Result<()> {
        let req = AckTaskRequest {
            task_id: task_id.to_string(),
            success,
            error: error.to_string(),
        };
        self.inner
            .ack_task(req)
            .await
            .map_err(|s| DataPlaneError::TransportError(format!("ack_task: {s}")))?;
        Ok(())
    }

    /// Issue an explicit `ClaimDisk` RPC. The wrapper-style request only
    /// carries `(disk_uuid, pool_uuid)`; role / force are inferred by the
    /// dataplane (single-host appliances claim every disk as Data and
    /// reject non-empty disks unconditionally — see `task_handlers::
    /// claim_disk`).
    pub async fn claim_disk(&mut self, disk_uuid: &str, pool_uuid: &str) -> Result<Disk> {
        let req = ClaimDiskRequest {
            disk_uuid: disk_uuid.to_string(),
            pool_uuid: pool_uuid.to_string(),
        };
        let resp = self
            .inner
            .claim_disk(req)
            .await
            .map_err(|s| DataPlaneError::TransportError(format!("claim_disk: {s}")))?;
        let inner = resp.into_inner();
        Ok(inner.disk.unwrap_or_default())
    }

    /// Mirror of `MetaService::DeleteDisk` for clean release.
    pub async fn delete_disk(&mut self, disk_uuid: &str) -> Result<()> {
        let req = DeleteDiskRequest {
            uuid: disk_uuid.to_string(),
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
