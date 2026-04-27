//! TCP gRPC server retained for the chunk-service surface.
//!
//! Architecture-v2 moves the bulk of the legacy "DataplaneService" RPC
//! surface (NVMe-oF target plumbing, lvm/lvol creation, NBD export, etc.)
//! out of this crate; what remains is the chunk-service which lets tools
//! and tests put / get / delete chunks against a registered
//! [`ChunkStore`].
//!
//! The data daemon's primary surface is the NDP UDS server (see
//! `ndp_server`); this TCP listener is kept for management / debug only
//! and is gated behind an explicit address being supplied.

use std::net::SocketAddr;
use std::sync::Arc;

use crate::backend::chunk_store::ChunkStore;
use crate::error::Result;
use crate::transport::chunk_proto::chunk_service_server::ChunkServiceServer;
use crate::transport::chunk_service::ChunkServiceImpl;

/// Configuration for the chunk-service TCP server.
pub struct GrpcServerConfig {
    pub listen_address: String,
    pub port: u16,
    /// Path to CA certificate for mTLS (empty = no TLS).
    pub tls_ca_cert: String,
    /// Path to server certificate.
    pub tls_server_cert: String,
    /// Path to server private key.
    pub tls_server_key: String,
}

impl Default for GrpcServerConfig {
    fn default() -> Self {
        Self {
            listen_address: "::".to_string(),
            port: 9500,
            tls_ca_cert: String::new(),
            tls_server_cert: String::new(),
            tls_server_key: String::new(),
        }
    }
}

/// Handle returned by [`GrpcServer::start`].
pub struct GrpcServerHandle {
    pub addr: SocketAddr,
    task: tokio::task::JoinHandle<()>,
}

impl GrpcServerHandle {
    pub fn shutdown(self) {
        self.task.abort();
    }
}

/// Chunk-service-only gRPC server.
pub struct GrpcServer {
    config: GrpcServerConfig,
    chunk_store: Arc<dyn ChunkStore>,
}

impl GrpcServer {
    pub fn new(config: GrpcServerConfig, chunk_store: Arc<dyn ChunkStore>) -> Self {
        Self {
            config,
            chunk_store,
        }
    }

    /// Bind the configured TCP address and start serving.
    pub async fn start(&self) -> Result<GrpcServerHandle> {
        let addr: SocketAddr = format!("{}:{}", self.config.listen_address, self.config.port)
            .parse()
            .map_err(|e| {
                crate::error::DataPlaneError::TransportError(format!("invalid listen address: {e}"))
            })?;

        let tls_config = if !self.config.tls_ca_cert.is_empty()
            && !self.config.tls_server_cert.is_empty()
            && !self.config.tls_server_key.is_empty()
        {
            let ca_cert = std::fs::read(&self.config.tls_ca_cert).map_err(|e| {
                crate::error::DataPlaneError::TransportError(format!(
                    "read CA cert {}: {e}",
                    self.config.tls_ca_cert
                ))
            })?;
            let server_cert = std::fs::read(&self.config.tls_server_cert).map_err(|e| {
                crate::error::DataPlaneError::TransportError(format!(
                    "read server cert {}: {e}",
                    self.config.tls_server_cert
                ))
            })?;
            let server_key = std::fs::read(&self.config.tls_server_key).map_err(|e| {
                crate::error::DataPlaneError::TransportError(format!(
                    "read server key {}: {e}",
                    self.config.tls_server_key
                ))
            })?;
            let identity = tonic::transport::Identity::from_pem(server_cert, server_key);
            let client_ca = tonic::transport::Certificate::from_pem(ca_cert);
            let tls = tonic::transport::ServerTlsConfig::new()
                .identity(identity)
                .client_ca_root(client_ca);
            log::info!("chunk-service TCP server TLS enabled (mTLS with client CA verification)");
            Some(tls)
        } else {
            log::warn!("chunk-service TCP server TLS DISABLED — no cert paths configured");
            None
        };

        let listener = tokio::net::TcpListener::bind(addr).await.map_err(|e| {
            crate::error::DataPlaneError::TransportError(format!("bind failed: {e}"))
        })?;
        let bound_addr = listener.local_addr().map_err(|e| {
            crate::error::DataPlaneError::TransportError(format!("local_addr: {e}"))
        })?;
        log::info!("chunk-service TCP server listening on {}", bound_addr);

        let store = self.chunk_store.clone();
        let task = tokio::spawn(async move {
            let incoming = tokio_stream::wrappers::TcpListenerStream::new(listener);
            let mut builder = tonic::transport::Server::builder();
            if let Some(tls) = tls_config {
                builder = builder.tls_config(tls).unwrap_or_else(|e| {
                    log::error!("chunk-service TLS config error: {}", e);
                    tonic::transport::Server::builder()
                });
            }
            builder
                .add_service(ChunkServiceServer::new(ChunkServiceImpl::new(store)))
                .serve_with_incoming(incoming)
                .await
                .unwrap_or_else(|e| log::error!("chunk-service TCP server error: {}", e));
        });

        Ok(GrpcServerHandle {
            addr: bound_addr,
            task,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_grpc_config() {
        let config = GrpcServerConfig::default();
        assert_eq!(config.listen_address, "::");
        assert_eq!(config.port, 9500);
    }
}
