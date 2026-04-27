//! Generated protobuf bindings for the canonical MetaService contract.
//!
//! `meta` is the wrapper-style MetaService client used by chunk_map_cache,
//! api_subscriber, and the heartbeat path. The proto file lives at
//! `storage/api/proto/meta/meta.proto` and is the single source of truth
//! across novanas-meta, novanas-frontend, and novanas-dataplane.

#[allow(clippy::all, missing_docs)]
pub mod meta {
    tonic::include_proto!("novanas.meta.v1");
}
