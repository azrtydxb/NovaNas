//! novanas-meta: single-host cold-path metadata daemon.
//!
//! This crate is the source of placement and lifecycle truth for the local
//! data daemon in architecture-v2. It subscribes to `novanas-api` for the
//! authoritative pool/disk/volume state, persists chunk maps in redb, runs
//! the CRUSH placement algorithm, and emits tasks for the data daemon.
//!
//! The crate is deliberately decoupled from the dataplane crate (which is
//! SPDK-gated and cannot be linked from non-Linux hosts).

pub mod api_client;
pub mod crush;
pub mod policy;
pub mod server;
pub mod store;
pub mod topology;
pub mod types;

/// Generated tonic types for the meta gRPC service.
pub mod proto {
    tonic::include_proto!("novanas.meta.v1");
}

/// Hard-coded chunk size: 4 MiB.
pub const CHUNK_SIZE_BYTES: u64 = 4 * 1024 * 1024;

/// Compute chunk count for a volume of `size_bytes`.
pub fn chunk_count_for(size_bytes: u64) -> u64 {
    if size_bytes == 0 {
        0
    } else {
        size_bytes.div_ceil(CHUNK_SIZE_BYTES)
    }
}

#[cfg(test)]
mod lib_tests {
    use super::*;

    #[test]
    fn chunk_count_zero_size() {
        assert_eq!(chunk_count_for(0), 0);
    }

    #[test]
    fn chunk_count_exact_chunk() {
        assert_eq!(chunk_count_for(CHUNK_SIZE_BYTES), 1);
    }

    #[test]
    fn chunk_count_partial_chunk() {
        assert_eq!(chunk_count_for(CHUNK_SIZE_BYTES + 1), 2);
    }

    #[test]
    fn chunk_count_many() {
        assert_eq!(chunk_count_for(10 * CHUNK_SIZE_BYTES), 10);
    }
}
