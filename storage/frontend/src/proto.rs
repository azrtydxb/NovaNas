//! Generated protobuf bindings.
//!
//! `meta` is the MetaService client used by chunk_map_cache, api_subscriber,
//! and the heartbeat path.
//! `chunk` is included for completeness; the frontend speaks NDP (not chunk
//! gRPC) on the I/O hot path, but the type definitions are useful for any
//! future control-plane RPCs.

#[allow(clippy::all, missing_docs)]
pub mod meta {
    tonic::include_proto!("novanas.meta.v1");
}

#[allow(clippy::all, missing_docs)]
pub mod chunk {
    tonic::include_proto!("chunk");
}
