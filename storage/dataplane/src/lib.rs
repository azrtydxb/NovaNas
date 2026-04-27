//! NovaNas data daemon (binary name `novanas-dataplane`, role
//! `novanas-data`).
//!
//! Owns disks and chunks on a single appliance host. Hosts the SPDK env
//! that the bdev layer drives, runs a chunk-store on every claimed disk,
//! and consumes work from `novanas-meta`'s task queue
//! (`MetaService::PollTasks`). See `docs/16-data-meta-frontend.md` for
//! the locked architecture.

pub mod backend;
#[cfg(feature = "spdk-sys")]
pub mod bdev;
pub mod chunk;
pub mod config;
pub mod crypto;
pub mod device;
pub mod disk_discovery;
pub mod error;
pub mod meta_client;
pub mod metadata;
pub mod openbao;
pub mod policy;
#[cfg(feature = "spdk-sys")]
pub mod spdk;
#[cfg(not(feature = "spdk-sys"))]
#[path = "bdev/sub_block.rs"]
pub mod sub_block;
pub mod task_handlers;
pub mod task_runner;
pub mod tracing_init;
pub mod transport;

/// Global tokio runtime handle — set by the binary entry point before SPDK init.
static TOKIO_HANDLE: std::sync::OnceLock<tokio::runtime::Handle> = std::sync::OnceLock::new();

/// Register the tokio runtime handle (called from main.rs).
pub fn set_tokio_handle(handle: tokio::runtime::Handle) {
    TOKIO_HANDLE.set(handle).expect("tokio handle already set");
}

/// Get the global tokio runtime handle.
pub fn tokio_handle() -> &'static tokio::runtime::Handle {
    TOKIO_HANDLE.get().expect("tokio runtime not initialized")
}
