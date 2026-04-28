//! SPDK environment + reactor + bdev management for the frontend.
//!
//! Frontend has its OWN SPDK env (separate process from the data daemon).
//! Modules are direct ports from `storage/dataplane/src/spdk/...`:
//!
//!  - `context`        — `Completion` + `AsyncCompletion` for SPDK callback↔caller sync
//!  - `reactor_dispatch` — `send_to_reactor` / `dispatch_sync` / `dispatch_async`
//!  - `env`            — `spdk_app_start` + iobuf tuning + reactor lifecycle
//!  - `bdev_manager`   — narrow bookkeeping for SPDK bdevs registered by the frontend
//!  - `volume_bdev`    — the custom `novanas_<volume>` bdev that fans I/O out to ChunkEngine

pub mod bdev_manager;
pub mod context;
pub mod env;
pub mod reactor_dispatch;
pub mod volume_bdev;

use crate::error::Result;

/// Initialise SPDK and run the reactor on the calling thread.
///
/// Blocks until `env::shutdown_spdk_env()` is called from a signal
/// handler (or from the tokio side when the daemon is shutting down).
pub fn run(reactor_mask: &str, mem_size_mb: u32, listen_port: u16) -> Result<()> {
    env::init_spdk_env(reactor_mask, mem_size_mb, listen_port)?;
    env::shutdown_spdk_env();
    Ok(())
}

/// Set the tokio handle used by the SPDK volume bdev callbacks. Must be
/// called once from the tokio runtime *before* SPDK starts dispatching
/// I/O.
pub fn set_tokio_handle(handle: tokio::runtime::Handle) {
    volume_bdev::set_tokio_handle_pub(handle);
}
