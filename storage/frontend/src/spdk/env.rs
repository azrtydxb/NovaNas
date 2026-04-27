//! SPDK environment lifecycle.
//!
//! NOTE on duplication-not-yet-resolved status: in the architecture-v2
//! split (docs/16) the frontend gets its own SPDK env, distinct from
//! data's. The full FFI scaffolding (bindgen, link-args, etc.) lives in
//! `storage/dataplane/build.rs`. Replicating that build pipeline in
//! the frontend crate is a non-trivial deduplication exercise and is
//! intentionally deferred to Agent C's `PR-DataConsolidate`. Until
//! then, this module declares the public surface and returns an
//! "unimplemented in frontend; route through the dataplane env for now"
//! error if the frontend is launched with `--features spdk-sys` against
//! a host that hasn't been wired through Agent C's port. This keeps
//! the API stable, lets the frontend compile under both feature
//! flavours, and documents the gap explicitly.

use crate::error::{FrontendError, Result};

pub fn init_spdk_env(reactor_mask: &str, mem_size_mb: u32, listen_port: u16) -> Result<()> {
    log::warn!(
        "frontend SPDK env: init({}, {} MB, port {}) requested but not yet linked; \
         see dataplane/src/spdk/env.rs (Agent C will move it)",
        reactor_mask,
        mem_size_mb,
        listen_port,
    );
    Err(FrontendError::SpdkInit(
        "frontend SPDK env not yet ported from dataplane; build with --no-default-features".into(),
    ))
}

pub fn shutdown_spdk_env() {
    // Nothing to tear down in the placeholder.
}
