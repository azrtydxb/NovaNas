//! Shard replicator — drives Reed-Solomon shard repair driven by
//! `PolicyEngine::drain_degraded_shards`.
//!
//! The bdev layer ([`crate::bdev::erasure::ErasureBdev`] under `spdk-sys`)
//! reports degraded / missing shards via
//! [`PolicyEngine::report_degraded_shard`]. A [`ShardReplicator`] worker wakes
//! on a tokio interval, drains the pending queue, looks up the erasure
//! geometry for each volume via a [`ShardGeometryProvider`], and asks the
//! `PolicyEngine` to reconstruct the shard onto a healthy CRUSH target.
//!
//! Decoupling the geometry lookup behind a trait keeps this module
//! feature-agnostic: in production the provider reads from the
//! `bdev::erasure` registry (requires `--features spdk-sys`); in unit tests
//! a simple `HashMap`-backed provider is used.
//!
//! # Contract
//!
//! - The worker drains at most `max_batch` reports per tick (default 64).
//! - Each reconstruction is synchronous within the tick (serial). If a
//!   reconstruction fails, the error is logged and the next report is
//!   processed — reports are NOT re-enqueued (the bdev will re-report on the
//!   next I/O probe).
//! - The worker exits cleanly when `stop()` is called or when the returned
//!   `JoinHandle` is dropped.

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use log::{debug, info, warn};
use tokio::sync::RwLock;
use tokio::task::JoinHandle;

use crate::policy::engine::{DegradedShardReport, PolicyEngine};

/// Shard geometry for a volume. Resolves a degraded shard report into the
/// concrete chunk_id and (data_shards, parity_shards) pair needed to drive
/// Reed-Solomon reconstruction.
#[derive(Debug, Clone)]
pub struct ShardGeometry {
    /// The chunk identifier used by the EC layout — for per-volume EC
    /// bdevs, this is typically the `volume_id` itself.
    pub chunk_id: String,
    pub data_shards: u32,
    pub parity_shards: u32,
}

/// Trait for resolving a `volume_id` to its erasure geometry.
///
/// Two production-grade implementations ship with this crate:
/// - [`ErasureBdevGeometryProvider`] (under `--features spdk-sys`) reads from
///   the global [`crate::bdev::erasure`] registry.
/// - [`StaticGeometryProvider`] is an in-memory map used by tests and by the
///   Go agent when it knows a volume's EC layout up-front.
pub trait ShardGeometryProvider: Send + Sync {
    fn lookup(&self, volume_id: &str) -> Option<ShardGeometry>;
}

/// Simple in-memory provider — useful for tests and for the Go agent to
/// populate via gRPC-set policies.
#[derive(Default)]
pub struct StaticGeometryProvider {
    inner: RwLock<HashMap<String, ShardGeometry>>,
}

impl StaticGeometryProvider {
    pub fn new() -> Self {
        Self::default()
    }

    pub async fn set(&self, volume_id: impl Into<String>, geom: ShardGeometry) {
        self.inner.write().await.insert(volume_id.into(), geom);
    }

    pub async fn remove(&self, volume_id: &str) {
        self.inner.write().await.remove(volume_id);
    }
}

impl ShardGeometryProvider for StaticGeometryProvider {
    fn lookup(&self, volume_id: &str) -> Option<ShardGeometry> {
        // Using try_read here keeps the trait synchronous. Readers are
        // non-blocking except during the very brief write windows from
        // `set`/`remove`; failing a tick is fine — the next tick retries.
        self.inner.try_read().ok()?.get(volume_id).cloned()
    }
}

/// Erasure geometry provider backed by the global
/// [`crate::bdev::erasure`] registry. Only available when the `spdk-sys`
/// feature is enabled (the bdev module is gated behind it).
#[cfg(feature = "spdk-sys")]
pub struct ErasureBdevGeometryProvider;

#[cfg(feature = "spdk-sys")]
impl ShardGeometryProvider for ErasureBdevGeometryProvider {
    fn lookup(&self, volume_id: &str) -> Option<ShardGeometry> {
        let bdev = crate::bdev::erasure::get_erasure_bdev(volume_id)?;
        Some(ShardGeometry {
            chunk_id: bdev.volume_id.clone(),
            data_shards: bdev.data_shards as u32,
            parity_shards: bdev.parity_shards as u32,
        })
    }
}

/// Configuration for the `ShardReplicator` worker.
#[derive(Debug, Clone)]
pub struct ShardReplicatorConfig {
    /// How often to drain the degraded queue. Default: 30s.
    pub interval: Duration,
    /// Maximum reports to process per tick. Default: 64.
    pub max_batch: usize,
}

impl Default for ShardReplicatorConfig {
    fn default() -> Self {
        Self {
            interval: Duration::from_secs(30),
            max_batch: 64,
        }
    }
}

/// Periodic worker that drains degraded shard reports and drives
/// Reed-Solomon reconstruction via the `PolicyEngine`.
pub struct ShardReplicator {
    engine: Arc<PolicyEngine>,
    geometry: Arc<dyn ShardGeometryProvider>,
    config: ShardReplicatorConfig,
}

impl ShardReplicator {
    pub fn new(
        engine: Arc<PolicyEngine>,
        geometry: Arc<dyn ShardGeometryProvider>,
        config: ShardReplicatorConfig,
    ) -> Self {
        Self {
            engine,
            geometry,
            config,
        }
    }

    /// Run one drain pass. Exposed for unit tests and for callers that want
    /// to drive the replicator manually (e.g. from a scheduler other than
    /// tokio).
    ///
    /// Returns `(attempted, succeeded)`: total reports dequeued and those
    /// that produced a successful reconstruction.
    pub async fn run_once(&self) -> (usize, usize) {
        let reports = self.engine.drain_degraded_shards(self.config.max_batch);
        if reports.is_empty() {
            return (0, 0);
        }

        let attempted = reports.len();
        let mut succeeded = 0usize;
        debug!(
            "shard replicator: processing {} degraded shard report(s)",
            attempted
        );

        for report in &reports {
            match self.process_report(report).await {
                Ok(target) => {
                    info!(
                        "shard replicator: repaired {}:shard:{} -> {}",
                        report.volume_id, report.shard_index, target
                    );
                    succeeded += 1;
                }
                Err(e) => {
                    warn!(
                        "shard replicator: failed to repair {}:shard:{}: {}",
                        report.volume_id, report.shard_index, e
                    );
                }
            }
        }

        (attempted, succeeded)
    }

    async fn process_report(&self, report: &DegradedShardReport) -> crate::error::Result<String> {
        let geom = self.geometry.lookup(&report.volume_id).ok_or_else(|| {
            crate::error::DataPlaneError::ChunkEngineError(format!(
                "no erasure geometry registered for volume {}",
                report.volume_id
            ))
        })?;

        self.engine
            .reconstruct_reported_shard(
                report,
                &geom.chunk_id,
                geom.data_shards,
                geom.parity_shards,
            )
            .await
    }

    /// Spawn a background tokio task that runs `run_once` on `config.interval`.
    /// Returns a handle that cancels the worker when dropped.
    pub fn spawn(self: Arc<Self>) -> ShardReplicatorHandle {
        let (stop_tx, mut stop_rx) = tokio::sync::oneshot::channel::<()>();
        let this = self.clone();
        let interval = self.config.interval;
        let task = tokio::spawn(async move {
            info!(
                "shard replicator started (interval={:?}, batch={})",
                interval, this.config.max_batch
            );
            let mut tick = tokio::time::interval(interval);
            // Skip the initial immediate tick so first work happens after
            // one full interval — matches tokio's default but stated explicitly.
            tick.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
            tick.tick().await;
            loop {
                tokio::select! {
                    _ = tick.tick() => {
                        let (att, ok) = this.run_once().await;
                        if att > 0 {
                            debug!(
                                "shard replicator tick: attempted={}, succeeded={}",
                                att, ok
                            );
                        }
                    }
                    _ = &mut stop_rx => {
                        info!("shard replicator stopping");
                        break;
                    }
                }
            }
        });
        ShardReplicatorHandle {
            task: Some(task),
            stop_tx: Some(stop_tx),
        }
    }
}

/// Handle to a spawned `ShardReplicator`. Dropping it cancels the worker.
pub struct ShardReplicatorHandle {
    task: Option<JoinHandle<()>>,
    stop_tx: Option<tokio::sync::oneshot::Sender<()>>,
}

impl ShardReplicatorHandle {
    /// Send a stop signal and wait for the worker task to exit.
    pub async fn shutdown(mut self) {
        if let Some(tx) = self.stop_tx.take() {
            let _ = tx.send(());
        }
        if let Some(task) = self.task.take() {
            let _ = task.await;
        }
    }
}

impl Drop for ShardReplicatorHandle {
    fn drop(&mut self) {
        if let Some(tx) = self.stop_tx.take() {
            let _ = tx.send(());
        }
        if let Some(task) = self.task.take() {
            task.abort();
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::backend::chunk_store::{ChunkHeader, ChunkStore};
    use crate::backend::file_store::FileChunkStore;
    use crate::metadata::topology::{Backend, BackendType, ClusterMap, Node, NodeStatus};
    use crate::policy::engine::DegradedShardState;
    use crate::policy::location_store::ChunkLocationStore;

    /// Single-node topology — forces CRUSH picks onto the only node. Used
    /// for tests that must stay on the local node so no gRPC is attempted.
    fn single_node_topology(node_id: &str) -> ClusterMap {
        let mut map = ClusterMap::new(1);
        map.add_node(Node {
            id: node_id.to_string(),
            address: "127.0.0.1".to_string(),
            port: 9100,
            backends: vec![Backend {
                id: format!("{node_id}-be"),
                node_id: node_id.to_string(),
                capacity_bytes: 1_000_000_000_000,
                used_bytes: 0,
                weight: 100,
                backend_type: BackendType::Bdev,
            }],
            status: NodeStatus::Online,
        });
        map
    }

    /// Reed-Solomon encode `payload` into `data_shards + parity_shards`
    /// equal-length byte vectors. Shard size is rounded up to the next
    /// even number (required by `reed-solomon-simd`).
    fn encode_payload(data_shards: u32, parity_shards: u32, payload: &[u8]) -> Vec<Vec<u8>> {
        let d = data_shards as usize;
        let p = parity_shards as usize;
        let raw_shard_size = payload.len().div_ceil(d);
        let shard_size = (raw_shard_size + 1) & !1;
        let padded_len = shard_size * d;
        let mut padded = payload.to_vec();
        padded.resize(padded_len, 0);
        let data: Vec<Vec<u8>> = padded.chunks(shard_size).map(|c| c.to_vec()).collect();
        let parity = reed_solomon_simd::encode(d, p, data.iter().map(|s| s.as_slice())).unwrap();
        let mut all = data;
        all.extend(parity);
        all
    }

    /// Decode the reverse — takes the available (idx, shard) set and
    /// reconstructs the original payload.
    fn decode_payload(
        data_shards: u32,
        parity_shards: u32,
        available: &[(usize, Vec<u8>)],
        original_len: usize,
    ) -> Vec<u8> {
        let d = data_shards as usize;
        let p = parity_shards as usize;
        let has_all_data = (0..d).all(|i| available.iter().any(|(idx, _)| *idx == i));
        if has_all_data {
            let mut out = Vec::with_capacity(original_len);
            for i in 0..d {
                let shard = available.iter().find(|(idx, _)| *idx == i).unwrap();
                out.extend_from_slice(&shard.1);
            }
            out.truncate(original_len);
            return out;
        }
        let mut originals: Vec<(usize, &[u8])> = Vec::new();
        let mut recovery: Vec<(usize, &[u8])> = Vec::new();
        for (idx, data) in available {
            if *idx < d {
                originals.push((*idx, data.as_slice()));
            } else {
                recovery.push((*idx - d, data.as_slice()));
            }
        }
        let recovered = reed_solomon_simd::decode(d, p, originals, recovery).unwrap();
        let mut out = Vec::with_capacity(original_len);
        for i in 0..d {
            if let Some((_, data)) = available.iter().find(|(idx, _)| *idx == i) {
                out.extend_from_slice(data);
            } else {
                out.extend_from_slice(recovered.get(&i).unwrap());
            }
        }
        out.truncate(original_len);
        out
    }

    fn wrap_shard(data: &[u8]) -> Vec<u8> {
        let header = ChunkHeader {
            magic: *b"NVAC",
            version: 1,
            flags: 0,
            checksum: crc32c::crc32c(data),
            data_len: data.len() as u32,
            _reserved: [0; 2],
        };
        let mut buf = Vec::with_capacity(ChunkHeader::SIZE + data.len());
        buf.extend_from_slice(&header.to_bytes());
        buf.extend_from_slice(data);
        buf
    }

    /// Build a replicator whose local node holds every surviving shard.
    /// Returns (engine, tmpdirs, replicator, geom_provider).
    async fn setup(
        data_shards: u32,
        parity_shards: u32,
        local_node: &str,
    ) -> (
        Arc<PolicyEngine>,
        tempfile::TempDir,
        tempfile::TempDir,
        Arc<ShardReplicator>,
        Arc<StaticGeometryProvider>,
        Arc<FileChunkStore>,
    ) {
        let store_dir = tempfile::tempdir().unwrap();
        let db_dir = tempfile::tempdir().unwrap();

        let file_store = Arc::new(
            FileChunkStore::new(store_dir.path().to_path_buf(), 64 * 1024 * 1024).unwrap(),
        );
        let location_store =
            Arc::new(ChunkLocationStore::open(db_dir.path().join("loc.redb")).unwrap());

        // Use a single-node topology so CRUSH always picks the local node;
        // this keeps reconstruction on the local `FileChunkStore` (no gRPC
        // connection attempts). The engine's `reconstruct_reported_shard`
        // will happily write the repaired shard back to the local node even
        // though it already holds other surviving shards — the
        // failure-domain preference relaxes to the top CRUSH pick when
        // the cluster is too small.
        let engine = Arc::new(PolicyEngine::new(
            local_node.to_string(),
            location_store,
            file_store.clone(),
            single_node_topology(local_node),
        ));

        let geom = Arc::new(StaticGeometryProvider::new());
        geom.set(
            "volA",
            ShardGeometry {
                chunk_id: "volA".to_string(),
                data_shards,
                parity_shards,
            },
        )
        .await;

        let replicator = Arc::new(ShardReplicator::new(
            engine.clone(),
            geom.clone(),
            ShardReplicatorConfig {
                interval: Duration::from_millis(10),
                max_batch: 16,
            },
        ));
        (engine, store_dir, db_dir, replicator, geom, file_store)
    }

    /// Happy path: a single data shard is missing, enough surviving shards
    /// are available locally for RS decode, and reconstruction writes the
    /// repaired shard to a healthy target.
    #[tokio::test]
    async fn happy_path_reconstructs_missing_data_shard() {
        let data_shards = 4u32;
        let parity_shards = 2u32;
        let (engine, _s, _d, replicator, _geom, file_store) =
            setup(data_shards, parity_shards, "node-0").await;

        // Build a 4+2 EC layout for "volA", encode a payload, and seed every
        // shard in the *local* FileChunkStore. We also record locations so
        // reconstruct_reported_shard can find them.
        let payload = vec![0x42u8; 2048];
        let shards = encode_payload(data_shards, parity_shards, &payload);

        // Seed shards 1..5 locally (shard 0 is the "missing" one).
        for (idx, shard) in shards.iter().enumerate() {
            if idx == 0 {
                continue;
            }
            let shard_id = format!("volA:shard:{idx}");
            file_store.put(&shard_id, &wrap_shard(shard)).await.unwrap();
            engine.record_chunk_location(&shard_id, "node-0").unwrap();
        }

        // Report shard 0 missing.
        engine.report_degraded_shard("volA", 0, DegradedShardState::Missing);
        assert_eq!(engine.pending_degraded_shards(), 1);

        let (attempted, succeeded) = replicator.run_once().await;
        assert_eq!(attempted, 1);
        assert_eq!(succeeded, 1, "happy path should succeed");

        // The reconstructed shard must now exist in the local store (the
        // CRUSH pick lands on the local node because surviving shards are
        // all on node-0 and CRUSH excludes those).
        let reconstructed_id = "volA:shard:0";
        let loc = engine
            .location_store()
            .get_location(reconstructed_id)
            .unwrap()
            .unwrap();
        assert!(
            !loc.node_ids.is_empty(),
            "reconstructed shard location must be recorded"
        );
        // And the repaired bytes must decode back to the original payload.
        let mut available: Vec<(usize, Vec<u8>)> = Vec::new();
        for idx in 0..6 {
            let shard_id = format!("volA:shard:{idx}");
            if let Ok(raw) = file_store.get(&shard_id).await {
                // Strip header.
                if raw.len() < ChunkHeader::SIZE {
                    continue;
                }
                let header_bytes: [u8; ChunkHeader::SIZE] =
                    raw[..ChunkHeader::SIZE].try_into().unwrap();
                let header = ChunkHeader::from_bytes(&header_bytes).unwrap();
                let body =
                    raw[ChunkHeader::SIZE..ChunkHeader::SIZE + header.data_len as usize].to_vec();
                available.push((idx, body));
            }
        }
        assert!(
            available.iter().any(|(i, _)| *i == 0),
            "reconstructed shard 0 must be readable from local store"
        );
        let decoded = decode_payload(data_shards, parity_shards, &available, payload.len());
        assert_eq!(decoded, payload);
    }

    /// Quorum-lost error path: fewer than `data_shards` surviving shards
    /// are reachable, so reconstruction must error out and no target is
    /// mutated.
    #[tokio::test]
    async fn quorum_lost_returns_error() {
        let data_shards = 4u32;
        let parity_shards = 2u32;
        let (engine, _s, _d, replicator, _geom, file_store) =
            setup(data_shards, parity_shards, "node-0").await;

        // Seed only 2 surviving shards — below the 4 required for decode.
        let payload = vec![0xAAu8; 1024];
        let shards = encode_payload(data_shards, parity_shards, &payload);

        for (idx, shard) in shards.iter().enumerate().take(2) {
            let shard_id = format!("volA:shard:{idx}");
            file_store.put(&shard_id, &wrap_shard(shard)).await.unwrap();
            engine.record_chunk_location(&shard_id, "node-0").unwrap();
        }

        // Report shard 5 missing — only 2 survivors, need 4.
        engine.report_degraded_shard("volA", 5, DegradedShardState::Missing);

        let (attempted, succeeded) = replicator.run_once().await;
        assert_eq!(attempted, 1);
        assert_eq!(succeeded, 0, "quorum-lost path must not succeed");

        // The degraded queue must have been drained (we don't re-enqueue).
        assert_eq!(engine.pending_degraded_shards(), 0);

        // No placement recorded for shard 5.
        let loc = engine
            .location_store()
            .get_location("volA:shard:5")
            .unwrap();
        assert!(loc.is_none(), "no placement should be recorded on failure");
    }

    /// Unknown volume: no geometry registered → report is drained and
    /// the error is logged, no panic.
    #[tokio::test]
    async fn unknown_volume_is_skipped() {
        let (engine, _s, _d, replicator, geom, _file) = setup(4, 2, "node-0").await;
        geom.remove("volA").await; // wipe out geometry

        engine.report_degraded_shard("volA", 0, DegradedShardState::Missing);
        let (attempted, succeeded) = replicator.run_once().await;
        assert_eq!(attempted, 1);
        assert_eq!(succeeded, 0);
    }

    /// Empty queue is a no-op.
    #[tokio::test]
    async fn empty_queue_is_noop() {
        let (_engine, _s, _d, replicator, _geom, _file) = setup(4, 2, "node-0").await;
        let (attempted, succeeded) = replicator.run_once().await;
        assert_eq!(attempted, 0);
        assert_eq!(succeeded, 0);
    }

    /// `spawn` / `shutdown` lifecycle works.
    #[tokio::test]
    async fn spawn_and_shutdown() {
        let (_engine, _s, _d, replicator, _geom, _file) = setup(4, 2, "node-0").await;
        let handle = replicator.spawn();
        // Let one tick fire.
        tokio::time::sleep(Duration::from_millis(50)).await;
        handle.shutdown().await;
    }
}
