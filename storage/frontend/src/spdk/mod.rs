//! SPDK environment + reactor + bdev management for the frontend.
//!
//! Frontend has its OWN SPDK env (separate from the data daemon's).
//! All four submodules below are direct ports from
//! `storage/dataplane/src/spdk/{env, reactor_dispatch, context,
//! bdev_manager}.rs` — verbatim copies of the parts needed for the
//! frontend's hot path. They compile only with `--features spdk-sys`.
//!
//! Until Agent C deletes the dataplane's copies the duplication is
//! deliberate and called out in the PR description.

pub mod bdev_manager;
pub mod context;
pub mod env;
pub mod reactor_dispatch;
pub mod volume_bdev;

use crate::error::Result;

/// Initialise SPDK and run the reactor until shutdown.
///
/// The actual implementation depends heavily on SPDK FFI, so we delegate
/// to `env::init_spdk_env`. On signal, `env::shutdown_spdk_env` is
/// called by `main`.
pub fn run(reactor_mask: &str, mem_size_mb: u32, listen_port: u16) -> Result<()> {
    env::init_spdk_env(reactor_mask, mem_size_mb, listen_port)?;
    env::shutdown_spdk_env();
    Ok(())
}
