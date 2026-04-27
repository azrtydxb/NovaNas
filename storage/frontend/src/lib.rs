//! novanas-frontend — the hot-path daemon.
//!
//! Architecture-v2 (docs/16): the frontend is the per-host SPDK daemon
//! that hosts the NVMe-oF target, runs each volume's bdev, and fans
//! per-I/O work out via NDP/UDS to `novanas-data` and chunk-map
//! lookups via gRPC/UDS to `novanas-meta`.
//!
//! Module map:
//!
//! - `error` — `FrontendError` and `Result` alias.
//! - `proto` — tonic-generated client stubs for `MetaService` and
//!   `ChunkService`.
//! - `meta_client` — UDS-backed `MetaClient` impl + trait abstraction.
//! - `chunk_map_cache` — local cache of `(volume, chunk_index) →
//!   chunk_id`, persisted in redb, repopulated from meta on miss.
//! - `ndp_client` — chunk-keyed wrapper around the NDP UDS protocol.
//! - `chunk_engine` — volume↔chunk math, write-back cache, splice on
//!   write.
//! - `write_cache` — sub-block write absorption (verbatim port from
//!   dataplane; intentionally duplicated until Agent C consolidates).
//! - `open_chunk` — append-only WAL-style chunk lifecycle (lean port
//!   from dataplane).
//! - `volume_bdev` — `VolumeBdevManager` trait + no-op double.
//! - `nvmf` — `NvmfTarget` trait + no-op double.
//! - `api_subscriber` — HTTP polling reconciler against `novanas-api`.
//! - `reconciler` — wire-up between API events, meta, bdev manager and
//!   NVMe-oF target.
//! - `spdk` (cfg `spdk-sys`) — SPDK env / reactor / bdev mgmt scaffold,
//!   placeholder ports of dataplane modules pending Agent C's split.
//! - `spdk_nvmf` (cfg `spdk-sys`) — SPDK-backed `NvmfTarget` impl
//!   (placeholder port).

pub mod api_subscriber;
pub mod chunk_engine;
pub mod chunk_map_cache;
pub mod error;
pub mod meta_client;
pub mod ndp_client;
pub mod nvmf;
pub mod open_chunk;
pub mod proto;
pub mod reconciler;
pub mod volume_bdev;
pub mod write_cache;

#[cfg(feature = "spdk-sys")]
pub mod spdk;
#[cfg(feature = "spdk-sys")]
pub mod spdk_nvmf;
