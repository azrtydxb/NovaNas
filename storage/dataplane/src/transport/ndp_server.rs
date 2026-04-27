//! NDP UDS server — the data daemon's chunk-service surface for the
//! frontend.
//!
//! In architecture-v2 the frontend daemon connects to data over a Unix
//! domain socket (default `/var/run/novanas/ndp.sock`) and exchanges chunk
//! payloads. Until the cross-process volume-bdev / chunk-engine handshake
//! lands in the frontend rebuild, this module exposes the existing
//! `ChunkService` gRPC surface (`chunk_service.proto`) over the UDS so
//! tooling and tests can put/get/delete chunks against a registered
//! [`ChunkStore`].
//!
//! The previous tokio-based sub-block NDP listener (which translated NDP
//! header ops into volume-bdev I/O on the local backend) is dead with the
//! volume bdev's move to `storage/frontend`; reintroducing a native NDP
//! protocol on top of `ChunkStore` is tracked as a follow-up.
//!
//! All work happens on the global tokio runtime; no SPDK reactor
//! interaction is performed here.

use std::path::{Path, PathBuf};
use std::sync::Arc;

use tokio::net::UnixListener;
use tokio_stream::wrappers::UnixListenerStream;

use crate::backend::chunk_store::ChunkStore;
use crate::error::{DataPlaneError, Result};
use crate::transport::chunk_proto::chunk_service_server::ChunkServiceServer;
use crate::transport::chunk_service::ChunkServiceImpl;

/// Default Unix socket path for local NDP connections.
pub const NDP_UNIX_SOCKET: &str = "/var/run/novanas/ndp.sock";

/// Configuration for the data-daemon NDP UDS server.
pub struct NdpServerConfig {
    pub unix_socket: PathBuf,
}

impl Default for NdpServerConfig {
    fn default() -> Self {
        Self {
            unix_socket: PathBuf::from(NDP_UNIX_SOCKET),
        }
    }
}

/// Handle returned by [`start_ndp_server`] for shutting the listener down.
pub struct NdpServerHandle {
    pub socket_path: PathBuf,
    task: tokio::task::JoinHandle<()>,
}

impl NdpServerHandle {
    /// Abort the listener task and unlink the UDS file.
    pub fn shutdown(self) {
        self.task.abort();
        let _ = std::fs::remove_file(&self.socket_path);
    }
}

/// Bind the NDP UDS at `config.unix_socket` and serve `ChunkService` gRPC
/// requests against `store`.
///
/// Creates the parent directory if missing and unlinks any stale socket
/// file. Returns once the listener is bound; the actual serving loop runs
/// on the supplied tokio handle.
pub async fn start_ndp_server(
    config: NdpServerConfig,
    store: Arc<dyn ChunkStore>,
) -> Result<NdpServerHandle> {
    if let Some(parent) = config.unix_socket.parent() {
        if !parent.as_os_str().is_empty() {
            std::fs::create_dir_all(parent).map_err(|e| {
                DataPlaneError::TransportError(format!(
                    "create NDP socket parent {}: {e}",
                    parent.display()
                ))
            })?;
        }
    }
    remove_if_exists(&config.unix_socket)?;

    let listener = UnixListener::bind(&config.unix_socket).map_err(|e| {
        DataPlaneError::TransportError(format!(
            "bind NDP UDS at {}: {e}",
            config.unix_socket.display()
        ))
    })?;
    log::info!(
        "NDP UDS server listening on {}",
        config.unix_socket.display()
    );

    let socket_path = config.unix_socket.clone();
    let task = tokio::spawn(async move {
        let incoming = UnixListenerStream::new(listener);
        let service = ChunkServiceServer::new(ChunkServiceImpl::new(store));
        if let Err(e) = tonic::transport::Server::builder()
            .add_service(service)
            .serve_with_incoming(incoming)
            .await
        {
            log::error!("NDP UDS server stopped: {e}");
        }
    });

    Ok(NdpServerHandle { socket_path, task })
}

fn remove_if_exists(path: &Path) -> Result<()> {
    match std::fs::remove_file(path) {
        Ok(()) => Ok(()),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(e) => Err(DataPlaneError::TransportError(format!(
            "remove stale UDS {}: {e}",
            path.display()
        ))),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::backend::chunk_store::{ChunkStore, ChunkStoreStats};
    use async_trait::async_trait;
    use std::sync::Mutex;

    /// Trivial in-memory ChunkStore for binding tests.
    struct MemStore {
        inner: Mutex<std::collections::HashMap<String, Vec<u8>>>,
    }
    impl MemStore {
        fn new() -> Self {
            Self {
                inner: Mutex::new(Default::default()),
            }
        }
    }
    #[async_trait]
    impl ChunkStore for MemStore {
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
                    DataPlaneError::ChunkEngineError(format!("chunk {chunk_id} not found"))
                })
        }
        async fn delete(&self, chunk_id: &str) -> Result<()> {
            self.inner.lock().unwrap().remove(chunk_id);
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

    #[tokio::test]
    async fn ndp_server_binds_and_unlinks() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("ndp.sock");
        let cfg = NdpServerConfig {
            unix_socket: sock.clone(),
        };
        let handle = start_ndp_server(cfg, Arc::new(MemStore::new()))
            .await
            .unwrap();
        assert!(sock.exists());
        handle.shutdown();
        // shutdown unlinks the socket file
        // Give the abort a moment to settle on slower runners.
        tokio::time::sleep(std::time::Duration::from_millis(20)).await;
        assert!(!sock.exists());
    }

    #[tokio::test]
    async fn ndp_server_replaces_stale_socket() {
        let dir = tempfile::tempdir().unwrap();
        let sock = dir.path().join("ndp.sock");
        // Create a stale socket file.
        std::fs::write(&sock, b"stale").unwrap();
        let cfg = NdpServerConfig {
            unix_socket: sock.clone(),
        };
        let handle = start_ndp_server(cfg, Arc::new(MemStore::new()))
            .await
            .unwrap();
        assert!(sock.exists());
        handle.shutdown();
    }
}
