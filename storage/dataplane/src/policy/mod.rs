//! Policy primitives owned by the data daemon.
//!
//! Architecture-v2 split:
//! - the policy CHECKER (engine, evaluator, location_store) lives in
//!   `storage/meta`; that's the daemon that decides what should happen,
//! - the policy MOVER (`operations`) lives here; it executes the chunk-level
//!   commands meta dispatches via `ChunkOpTask`.

pub mod operations;
pub mod types;
