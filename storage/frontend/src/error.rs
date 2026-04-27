//! Error types for the novanas-frontend daemon.
//!
//! Frontend errors fall into a few categories:
//!  - control-plane (meta gRPC, API HTTP) — recoverable, retried by callers
//!  - data-plane (NDP I/O to data) — surface to the SPDK bdev as I/O error
//!  - SPDK / bdev / NVMe-oF target failures — fatal during startup,
//!    surfaced as control errors during operation
//!
//! All variants intentionally carry a String message rather than a structured
//! cause chain — the frontend's job is to log + propagate, not to introspect.

use thiserror::Error;

#[derive(Error, Debug)]
pub enum FrontendError {
    #[error("meta client error: {0}")]
    Meta(String),
    #[error("API subscriber error: {0}")]
    Api(String),
    #[error("NDP client error: {0}")]
    Ndp(String),
    #[error("chunk engine error: {0}")]
    Chunk(String),
    #[error("chunk-map cache error: {0}")]
    ChunkMapCache(String),
    #[error("NVMe-oF target error: {0}")]
    Nvmf(String),
    #[error("volume bdev error: {0}")]
    Bdev(String),
    #[error("SPDK init failed: {0}")]
    SpdkInit(String),
    #[error("invalid configuration: {0}")]
    Config(String),
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),
    #[error("serialization error: {0}")]
    Serde(#[from] serde_json::Error),
}

impl From<tonic::Status> for FrontendError {
    fn from(s: tonic::Status) -> Self {
        FrontendError::Meta(format!("gRPC {}: {}", s.code(), s.message()))
    }
}

impl From<tonic::transport::Error> for FrontendError {
    fn from(e: tonic::transport::Error) -> Self {
        FrontendError::Meta(format!("gRPC transport: {}", e))
    }
}

impl From<reqwest::Error> for FrontendError {
    fn from(e: reqwest::Error) -> Self {
        FrontendError::Api(e.to_string())
    }
}

impl From<ndp::NdpError> for FrontendError {
    fn from(e: ndp::NdpError) -> Self {
        FrontendError::Ndp(e.to_string())
    }
}

pub type Result<T> = std::result::Result<T, FrontendError>;
