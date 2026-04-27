//! SPDK reactor dispatch (placeholder port).
//!
//! Dispatching closures onto the SPDK app thread requires the full FFI
//! environment. Until Agent C resolves the duplication-not-yet-resolved
//! split, this module exposes a single `dispatch_sync` that runs the
//! closure inline on the calling thread. That is incorrect for live
//! SPDK I/O but keeps the API surface in place; production wiring goes
//! through `dataplane/src/spdk/reactor_dispatch.rs` for now.

use crate::error::Result;

pub fn dispatch_sync<F, T>(f: F) -> Result<T>
where
    F: FnOnce() -> Result<T> + Send + 'static,
    T: Send + 'static,
{
    f()
}
