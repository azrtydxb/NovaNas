//! Metadata building blocks — CRUSH placement, types, topology, store.
//!
//! These are the Rust types the future `novanas-meta` daemon will be
//! built on. Single-host, no Raft. The previous `raft_store` and
//! `raft_types` modules were deleted with the architecture-v2 strip;
//! see docs/16-data-meta-frontend.md.

pub mod crush;
pub mod store;
pub mod topology;
pub mod types;
