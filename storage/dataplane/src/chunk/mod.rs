//! Server-side chunk plumbing for the data daemon.
//!
//! The volume↔chunk math, write coalescing, NDP client, and ChunkEngine all
//! moved to `storage/frontend` in architecture-v2. What remains here is the
//! server-side machinery: the SPDK reactor NDP dispatcher and the
//! background bitmap-destage task.

#[cfg(feature = "spdk-sys")]
pub mod reactor_ndp;
pub mod sync;
