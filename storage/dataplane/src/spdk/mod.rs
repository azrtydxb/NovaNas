//! SPDK application lifecycle and reactor management.
//!
//! `nbd_manager` and `nvmf_manager` were removed in the architecture-v2
//! split: NBD bootstrap is the meta daemon's concern (and uses ndp instead
//! today), and the NVMe-oF target is the frontend daemon's concern.

pub mod bdev_manager;
pub mod context;
pub mod env;
pub mod reactor_dispatch;

use crate::config::DataPlaneConfig;
use crate::error::Result;

/// Run the SPDK data plane application.
///
/// `init_spdk_env` calls `spdk_app_start` which invokes the startup callback
/// (initialising SPDK managers) and then blocks in the SPDK reactor loop
/// until `spdk_app_stop` is called (e.g. via SIGINT). All higher-level
/// services (meta client, task runner, NDP server) are started by the
/// binary entry point in `main.rs` before this call.
pub fn run(config: DataPlaneConfig) -> Result<()> {
    env::init_spdk_env(&config)?;
    env::shutdown_spdk_env();
    Ok(())
}
