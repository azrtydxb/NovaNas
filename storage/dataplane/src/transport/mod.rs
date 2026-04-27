//! Transport layer.
//!
//! In architecture-v2 the data daemon's outward gRPC surface is much
//! smaller than v1's: the bulk DataplaneService has been retired with the
//! removal of the volume bdev, NVMe-oF target, and lvm/lvol RPCs. What
//! remains is:
//!   * `chunk_service` — gRPC service over a [`ChunkStore`].
//!   * `ndp_server` — the UDS chunk surface the frontend connects to.
//!   * `server` — optional TCP listener for the chunk service (debug /
//!     management).

pub mod chunk_service;
pub mod ndp_server;
pub mod server;

pub mod chunk_proto {
    tonic::include_proto!("chunk");
}

/// Tonic-compiled `MetaService` client (and message types) generated from
/// `storage/api/proto/meta/meta.proto`.
pub mod meta_proto {
    tonic::include_proto!("novanas.meta.v1");
}
