//! Policy engine — orchestrates chunk health evaluation and corrective actions.
//!
//! The `PolicyEngine` ties together the location store (where chunks live),
//! the evaluator (what needs fixing), and chunk operations (how to fix it).

use std::collections::{HashMap, HashSet, VecDeque};
use std::sync::{Arc, Mutex};

use log::{info, warn};
use tokio::sync::RwLock;

use crate::backend::chunk_store::ChunkStore;
use crate::chunk::engine::ChunkEngine;
use crate::error::Result;
use crate::metadata::topology::ClusterMap;
use crate::policy::evaluator::PolicyEvaluator;
use crate::policy::location_store::ChunkLocationStore;
use crate::policy::operations::ChunkOperations;
use crate::policy::types::*;

/// The policy engine orchestrates chunk health evaluation and corrective actions.
///
/// It holds the cluster topology, per-volume policies, and the persistent
/// location store.  The [`reconcile`](PolicyEngine::reconcile) method runs one
/// pass: evaluate every tracked chunk against its policy and execute any
/// corrective actions (replicate / remove-replica).
pub struct PolicyEngine {
    node_id: String,
    location_store: Arc<ChunkLocationStore>,
    operations: ChunkOperations,
    topology: RwLock<Arc<ClusterMap>>,
    policies: RwLock<HashMap<String, VolumePolicy>>,
    /// Optional ChunkEngine reference for migration I/O.
    chunk_engine: RwLock<Option<Arc<ChunkEngine>>>,
    /// Rate-limits concurrent migration tasks.
    migration_semaphore: Arc<tokio::sync::Semaphore>,
    /// Pending degraded shard reports awaiting reconcile.
    /// Keyed by (volume_id, shard_index); VecDeque preserves arrival order
    /// so the reconcile loop can drain oldest first.
    degraded_shards: Mutex<VecDeque<DegradedShardReport>>,
    /// Set of shard keys currently queued to avoid duplicate enqueues.
    degraded_shard_index: Mutex<HashSet<(String, usize)>>,
}

/// A report of a shard that has transitioned to a non-healthy state.
///
/// Emitted by bdev layer (e.g. `ErasureBdev::mark_shard_degraded`) and
/// drained by the reconcile loop to schedule repair actions.
#[derive(Debug, Clone)]
pub struct DegradedShardReport {
    pub volume_id: String,
    pub shard_index: usize,
    pub state: DegradedShardState,
    pub reported_at: std::time::SystemTime,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DegradedShardState {
    Degraded,
    Missing,
}

impl PolicyEngine {
    /// Create a new policy engine.
    ///
    /// * `node_id` — identifier for this node (used by `ChunkOperations` to
    ///   decide whether I/O is local or remote).
    /// * `location_store` — persistent store tracking chunk-to-node mappings.
    /// * `local_store` — the local chunk store for same-node I/O.
    /// * `topology` — initial cluster topology snapshot.
    pub fn new(
        node_id: String,
        location_store: Arc<ChunkLocationStore>,
        local_store: Arc<dyn ChunkStore>,
        topology: ClusterMap,
    ) -> Self {
        let operations = ChunkOperations::new(node_id.clone(), local_store);
        Self {
            node_id,
            location_store,
            operations,
            topology: RwLock::new(Arc::new(topology)),
            policies: RwLock::new(HashMap::new()),
            chunk_engine: RwLock::new(None),
            migration_semaphore: Arc::new(tokio::sync::Semaphore::new(16)),
            degraded_shards: Mutex::new(VecDeque::new()),
            degraded_shard_index: Mutex::new(HashSet::new()),
        }
    }

    /// Report that an erasure shard has become degraded or missing.
    ///
    /// Called by the bdev layer when I/O probes detect shard health changes.
    /// Reports are deduped by (volume_id, shard_index) so repeated callers
    /// do not flood the queue; the state of the latest report wins if the
    /// shard is already queued.
    ///
    /// The reconcile loop drains this queue via
    /// [`drain_degraded_shards`](Self::drain_degraded_shards) and emits
    /// `ReconstructShard` actions for each entry.
    pub fn report_degraded_shard(
        &self,
        volume_id: &str,
        shard_index: usize,
        state: DegradedShardState,
    ) {
        let key = (volume_id.to_string(), shard_index);
        let mut seen = self.degraded_shard_index.lock().unwrap();
        if !seen.insert(key.clone()) {
            // Already queued — update the state of the existing entry if the
            // new report is worse (Degraded -> Missing).
            let mut q = self.degraded_shards.lock().unwrap();
            if let Some(existing) = q
                .iter_mut()
                .find(|r| r.volume_id == key.0 && r.shard_index == shard_index)
            {
                if state == DegradedShardState::Missing
                    && existing.state == DegradedShardState::Degraded
                {
                    existing.state = DegradedShardState::Missing;
                    existing.reported_at = std::time::SystemTime::now();
                }
            }
            return;
        }
        let mut q = self.degraded_shards.lock().unwrap();
        q.push_back(DegradedShardReport {
            volume_id: key.0,
            shard_index,
            state,
            reported_at: std::time::SystemTime::now(),
        });
        info!(
            "degraded shard reported: volume={} shard={} state={:?}",
            volume_id, shard_index, state
        );
    }

    /// Drain up to `max` pending degraded-shard reports.
    ///
    /// Returned reports are removed from the internal queue; the reconcile
    /// loop owns the lifecycle after this call.
    pub fn drain_degraded_shards(&self, max: usize) -> Vec<DegradedShardReport> {
        let mut q = self.degraded_shards.lock().unwrap();
        let take = q.len().min(max);
        let drained: Vec<_> = q.drain(..take).collect();
        drop(q);
        if !drained.is_empty() {
            let mut seen = self.degraded_shard_index.lock().unwrap();
            for r in &drained {
                seen.remove(&(r.volume_id.clone(), r.shard_index));
            }
        }
        drained
    }

    /// Number of degraded-shard reports awaiting reconcile.
    pub fn pending_degraded_shards(&self) -> usize {
        self.degraded_shards.lock().unwrap().len()
    }

    /// Register or update a volume's replication policy.
    pub async fn set_policy(&self, policy: VolumePolicy) {
        let mut policies = self.policies.write().await;
        info!(
            "policy set for volume {}: desired_replicas={}",
            policy.volume_id, policy.desired_replicas
        );
        policies.insert(policy.volume_id.clone(), policy);
    }

    /// Retrieve the current policy for a volume, if any.
    pub async fn get_policy(&self, volume_id: &str) -> Option<VolumePolicy> {
        let policies = self.policies.read().await;
        policies.get(volume_id).cloned()
    }

    /// Replace the cluster topology with an updated snapshot.
    pub async fn update_topology(&self, topology: ClusterMap) {
        let mut topo = self.topology.write().await;
        info!(
            "topology updated: generation {} -> {}",
            topo.generation(),
            topology.generation()
        );
        *topo = Arc::new(topology);
    }

    /// Record that a chunk is stored on a given node.
    pub fn record_chunk_location(&self, chunk_id: &str, node_id: &str) -> Result<()> {
        self.location_store.add_node_to_chunk(chunk_id, node_id)
    }

    /// Record that a volume references a chunk.
    pub fn record_chunk_ref(&self, chunk_id: &str, volume_id: &str) -> Result<()> {
        self.location_store.add_volume_ref(chunk_id, volume_id)
    }

    /// Returns this engine's node ID.
    pub fn node_id(&self) -> &str {
        &self.node_id
    }

    /// Returns a handle to the persistent location store. The shard
    /// replicator and observability RPCs need direct access to enumerate
    /// locations without going through the reconcile loop.
    pub fn location_store(&self) -> &Arc<ChunkLocationStore> {
        &self.location_store
    }

    /// Set the ChunkEngine reference for migration I/O.
    /// Called after both PolicyEngine and ChunkEngine are created.
    pub async fn set_chunk_engine(&self, engine: Arc<ChunkEngine>) {
        let mut ce = self.chunk_engine.write().await;
        *ce = Some(engine);
    }

    /// Detect chunks stored locally that CRUSH says should be elsewhere.
    /// Returns a list of (volume_id, chunk_index, target_node) tuples.
    pub async fn detect_misplaced_chunks(&self) -> Vec<(String, u64, String)> {
        let mut misplaced = Vec::new();

        #[cfg(feature = "spdk-sys")]
        {
            let store = match crate::bdev::novanas_bdev::get_metadata_store() {
                Some(s) => s,
                None => return misplaced,
            };
            let topo = self.topology.read().await;
            let volumes = match store.list_volumes() {
                Ok(v) => v,
                Err(_) => return misplaced,
            };

            for vol in &volumes {
                // Skip deleting volumes
                if vol.status == crate::metadata::types::VolumeStatus::Deleting {
                    continue;
                }
                let factor = match vol.protection {
                    crate::metadata::types::Protection::Replication { factor } => factor as usize,
                    crate::metadata::types::Protection::ErasureCoding {
                        data_shards,
                        parity_shards,
                    } => (data_shards + parity_shards) as usize,
                };
                let chunks = match store.list_chunk_map(&vol.id) {
                    Ok(c) => c,
                    Err(_) => continue,
                };
                for cm in &chunks {
                    // Only migrate chunks we actually have locally
                    if !cm.placements.contains(&self.node_id) {
                        continue;
                    }
                    let chunk_key = format!("{}:{}", vol.id, cm.chunk_index);
                    let placements = crate::metadata::crush::select(&chunk_key, factor, &topo);
                    let still_here = placements.iter().any(|(n, _)| n == &self.node_id);
                    if !still_here {
                        if let Some((target, _)) = placements.first() {
                            misplaced.push((vol.id.clone(), cm.chunk_index, target.clone()));
                        }
                    }
                }
            }
        }

        misplaced
    }

    /// Run one reconciliation pass: evaluate all chunks, execute corrective
    /// actions.  Returns the number of actions attempted.
    ///
    /// Individual action failures are logged as warnings but do not abort the
    /// pass — the engine processes every action and returns the total count so
    /// the caller knows how many corrections were needed.
    pub async fn reconcile(&self) -> Result<usize> {
        let topology = self.topology.read().await;
        let policies = self.policies.read().await;

        // Load all chunk locations from persistent store.
        let locations = self.location_store.list_locations()?;

        // Build the chunk-ref map (chunk_id -> ChunkRef).
        let mut refs: HashMap<String, ChunkRef> = HashMap::new();
        for loc in &locations {
            if let Ok(Some(chunk_ref)) = self.location_store.get_ref(&loc.chunk_id) {
                refs.insert(loc.chunk_id.clone(), chunk_ref);
            }
        }

        // Evaluate all chunks against their policies.
        let evaluator = PolicyEvaluator::new(&topology);
        let actions = evaluator.evaluate_all(&locations, &policies, &refs);

        let action_count = actions.len();
        if action_count > 0 {
            info!(
                "reconcile: {} corrective action(s) to execute",
                action_count
            );
        }

        // Execute each action.  Failures are logged but do not stop the pass.
        for action in &actions {
            match action {
                PolicyAction::Replicate {
                    chunk_id,
                    source_node,
                    target_node,
                } => {
                    match self
                        .operations
                        .replicate_chunk(chunk_id, source_node, target_node)
                        .await
                    {
                        Ok(()) => {
                            if let Err(e) =
                                self.location_store.add_node_to_chunk(chunk_id, target_node)
                            {
                                warn!("failed to record chunk location for {}: {}", chunk_id, e);
                            }
                            info!(
                                "replicated chunk {} from {} to {}",
                                chunk_id, source_node, target_node
                            );
                        }
                        Err(e) => {
                            warn!("failed to replicate chunk {}: {}", chunk_id, e);
                        }
                    }
                }
                PolicyAction::RemoveReplica { chunk_id, node_id } => {
                    match self.operations.remove_replica(chunk_id, node_id).await {
                        Ok(()) => {
                            if let Err(e) = self
                                .location_store
                                .remove_node_from_chunk(chunk_id, node_id)
                            {
                                warn!("failed to remove chunk location for {}: {}", chunk_id, e);
                            }
                            info!("removed replica of chunk {} from {}", chunk_id, node_id);
                        }
                        Err(e) => {
                            warn!(
                                "failed to remove replica of chunk {} from {}: {}",
                                chunk_id, node_id, e
                            );
                        }
                    }
                }
                PolicyAction::ReconstructShard {
                    chunk_id,
                    shard_index,
                    data_shards,
                    parity_shards,
                    source_nodes: _,
                    target_node,
                } => {
                    // Build (shard_idx, node_id) for all surviving shards by
                    // scanning the location store.
                    let total = (*data_shards + *parity_shards) as usize;
                    let mut surviving: Vec<(usize, String)> = Vec::new();
                    for idx in 0..total {
                        if idx == *shard_index {
                            continue; // This is the missing one.
                        }
                        let shard_id = format!("{chunk_id}:shard:{idx}");
                        if let Ok(Some(loc)) = self.location_store.get_location(&shard_id) {
                            if let Some(nid) = loc.node_ids.first() {
                                surviving.push((idx, nid.clone()));
                            }
                        }
                    }

                    match self
                        .operations
                        .reconstruct_shard(
                            chunk_id,
                            *shard_index,
                            *data_shards,
                            *parity_shards,
                            &surviving,
                            target_node,
                        )
                        .await
                    {
                        Ok(()) => {
                            let shard_id = format!("{chunk_id}:shard:{shard_index}");
                            if let Err(e) = self
                                .location_store
                                .add_node_to_chunk(&shard_id, target_node)
                            {
                                warn!("failed to record shard location for {}: {}", shard_id, e);
                            }
                            info!(
                                "reconstructed EC shard {} index {} on {}",
                                chunk_id, shard_index, target_node
                            );
                        }
                        Err(e) => {
                            warn!(
                                "failed to reconstruct EC shard {} index {}: {}",
                                chunk_id, shard_index, e
                            );
                        }
                    }
                }
            }
        }

        // Migration check — detect and migrate misplaced chunks.
        {
            let chunk_engine = self.chunk_engine.read().await;
            if let Some(engine) = chunk_engine.as_ref() {
                let misplaced = self.detect_misplaced_chunks().await;
                if !misplaced.is_empty() {
                    info!(
                        "policy: {} misplaced chunks, migrating (max 16 concurrent)",
                        misplaced.len()
                    );
                    for (vol_id, chunk_idx, target) in misplaced {
                        let permit = self.migration_semaphore.clone().acquire_owned().await;
                        if permit.is_err() {
                            break;
                        }
                        let permit = permit.unwrap();
                        let engine = engine.clone();
                        tokio::spawn(async move {
                            if let Err(e) = engine.migrate_chunk(&vol_id, chunk_idx, &target).await
                            {
                                warn!(
                                    "migration {}:{} -> {} failed: {}",
                                    vol_id, chunk_idx, target, e
                                );
                            }
                            drop(permit);
                        });
                    }
                }
            }
        }

        Ok(action_count)
    }

    /// Reconstruct a reported degraded shard.
    ///
    /// Looks up surviving shards in the location store, picks a healthy
    /// target via CRUSH (avoiding nodes that already hold surviving shards),
    /// runs Reed-Solomon decode / re-encode (via
    /// [`ChunkOperations::reconstruct_shard`]), writes the reconstructed shard,
    /// and records the new placement.
    ///
    /// Returns the target node ID where the shard was placed on success.
    ///
    /// Fails fast with an error when fewer than `data_shards` surviving
    /// shards are resolvable (quorum lost) — the caller (e.g. the
    /// [`ShardReplicator`](crate::policy::shard_replicator::ShardReplicator)
    /// worker) will retry on the next tick.
    pub async fn reconstruct_reported_shard(
        &self,
        report: &DegradedShardReport,
        chunk_id: &str,
        data_shards: u32,
        parity_shards: u32,
    ) -> Result<String> {
        let topology = self.topology.read().await;
        let total = (data_shards + parity_shards) as usize;

        // Collect (idx, node_id) for each surviving shard from the location
        // store. A shard is considered surviving if any node in its
        // ChunkLocation list exists in the current topology.
        let mut surviving: Vec<(usize, String)> = Vec::new();
        let mut excluded_nodes: HashSet<String> = HashSet::new();
        for idx in 0..total {
            if idx == report.shard_index {
                continue;
            }
            let shard_id = format!("{chunk_id}:shard:{idx}");
            if let Ok(Some(loc)) = self.location_store.get_location(&shard_id) {
                for nid in &loc.node_ids {
                    if topology.get_node(nid).is_some() {
                        surviving.push((idx, nid.clone()));
                        excluded_nodes.insert(nid.clone());
                        break;
                    }
                }
            }
        }

        if surviving.len() < data_shards as usize {
            return Err(crate::error::DataPlaneError::ChunkEngineError(format!(
                "quorum lost for {}:shard:{}: only {} of {} data shards reachable",
                chunk_id,
                report.shard_index,
                surviving.len(),
                data_shards
            )));
        }

        // Pick a healthy target via CRUSH. We prefer a node that does not
        // already hold a shard of this chunk (to preserve failure-domain
        // diversity), but fall back to the top CRUSH pick if the cluster is
        // too small to satisfy that — better to repair onto a node that
        // already hosts a different shard than to leave the chunk broken.
        let crush_key = format!("{chunk_id}:shard:{}", report.shard_index);
        let candidates = crate::metadata::crush::select(&crush_key, total * 2, &topology);
        if candidates.is_empty() {
            return Err(crate::error::DataPlaneError::ChunkEngineError(format!(
                "no CRUSH candidates available for {}:shard:{}",
                chunk_id, report.shard_index
            )));
        }
        let target_node_id = candidates
            .iter()
            .map(|(nid, _)| nid.clone())
            .find(|nid| !excluded_nodes.contains(nid))
            .unwrap_or_else(|| candidates[0].0.clone());

        // Drop the topology read lock before the (potentially slow) I/O.
        drop(topology);

        self.operations
            .reconstruct_shard(
                chunk_id,
                report.shard_index,
                data_shards,
                parity_shards,
                &surviving,
                &target_node_id,
            )
            .await?;

        let shard_id = format!("{chunk_id}:shard:{}", report.shard_index);
        self.location_store
            .add_node_to_chunk(&shard_id, &target_node_id)?;

        info!(
            "shard replicator: reconstructed {} on {} (data={}, parity={})",
            shard_id, target_node_id, data_shards, parity_shards
        );
        Ok(target_node_id)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::backend::chunk_store::ChunkHeader;
    use crate::backend::file_store::FileChunkStore;
    use crate::metadata::topology::*;

    fn test_topology() -> ClusterMap {
        let mut map = ClusterMap::new(1);
        for i in 0..4 {
            map.add_node(Node {
                id: format!("node-{i}"),
                address: format!("10.0.0.{}", i + 1),
                port: 9100,
                backends: vec![Backend {
                    id: format!("bdev-{i}"),
                    node_id: format!("node-{i}"),
                    capacity_bytes: 1_000_000_000_000,
                    used_bytes: 0,
                    weight: 100,
                    backend_type: BackendType::Bdev,
                }],
                status: NodeStatus::Online,
            });
        }
        map
    }

    fn fake_chunk_id() -> String {
        "aabbccdd00112233aabbccdd00112233aabbccdd00112233aabbccdd00112233".to_string()
    }

    fn make_chunk_data(data: &[u8]) -> Vec<u8> {
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

    async fn make_engine() -> (tempfile::TempDir, tempfile::TempDir, PolicyEngine) {
        let store_dir = tempfile::tempdir().unwrap();
        let db_dir = tempfile::tempdir().unwrap();

        let file_store =
            FileChunkStore::new(store_dir.path().to_path_buf(), 64 * 1024 * 1024).unwrap();
        let location_store =
            Arc::new(ChunkLocationStore::open(db_dir.path().join("locations.redb")).unwrap());
        let topology = test_topology();

        let engine = PolicyEngine::new(
            "node-0".to_string(),
            location_store,
            Arc::new(file_store),
            topology,
        );

        (store_dir, db_dir, engine)
    }

    #[tokio::test]
    async fn degraded_shards_are_queued_and_deduped() {
        let (_s, _d, engine) = make_engine().await;

        // Empty to start.
        assert_eq!(engine.pending_degraded_shards(), 0);

        engine.report_degraded_shard("vol-a", 0, DegradedShardState::Degraded);
        engine.report_degraded_shard("vol-a", 1, DegradedShardState::Degraded);
        assert_eq!(engine.pending_degraded_shards(), 2);

        // Duplicate (same key) should not create a new entry but can upgrade
        // state from Degraded -> Missing.
        engine.report_degraded_shard("vol-a", 0, DegradedShardState::Missing);
        assert_eq!(engine.pending_degraded_shards(), 2);

        let drained = engine.drain_degraded_shards(10);
        assert_eq!(drained.len(), 2);
        // First drain honours FIFO — vol-a/0 was reported first.
        assert_eq!(drained[0].volume_id, "vol-a");
        assert_eq!(drained[0].shard_index, 0);
        assert_eq!(drained[0].state, DegradedShardState::Missing);
        assert_eq!(drained[1].shard_index, 1);
        assert_eq!(drained[1].state, DegradedShardState::Degraded);

        // Index also cleared, so a re-report now enqueues again.
        assert_eq!(engine.pending_degraded_shards(), 0);
        engine.report_degraded_shard("vol-a", 0, DegradedShardState::Degraded);
        assert_eq!(engine.pending_degraded_shards(), 1);
    }

    #[tokio::test]
    async fn drain_respects_max() {
        let (_s, _d, engine) = make_engine().await;
        for i in 0..5 {
            engine.report_degraded_shard("vol-b", i, DegradedShardState::Degraded);
        }
        let first = engine.drain_degraded_shards(2);
        assert_eq!(first.len(), 2);
        assert_eq!(engine.pending_degraded_shards(), 3);
        let rest = engine.drain_degraded_shards(100);
        assert_eq!(rest.len(), 3);
        assert_eq!(engine.pending_degraded_shards(), 0);
    }

    #[tokio::test]
    async fn set_and_get_policy() {
        let (_s, _d, engine) = make_engine().await;

        // No policy initially.
        assert!(engine.get_policy("vol-1").await.is_none());

        // Set a policy.
        engine
            .set_policy(VolumePolicy::new("vol-1".to_string(), 3))
            .await;
        let policy = engine.get_policy("vol-1").await.unwrap();
        assert_eq!(policy.desired_replicas, 3);

        // Update the same policy.
        engine
            .set_policy(VolumePolicy::new("vol-1".to_string(), 5))
            .await;
        let policy = engine.get_policy("vol-1").await.unwrap();
        assert_eq!(policy.desired_replicas, 5);
    }

    #[tokio::test]
    async fn record_and_query_locations() {
        let (_s, _d, engine) = make_engine().await;
        let chunk_id = fake_chunk_id();

        // Record locations.
        engine.record_chunk_location(&chunk_id, "node-0").unwrap();
        engine.record_chunk_location(&chunk_id, "node-1").unwrap();

        // Verify via the location store.
        let loc = engine
            .location_store
            .get_location(&chunk_id)
            .unwrap()
            .unwrap();
        assert_eq!(loc.node_ids.len(), 2);
        assert!(loc.node_ids.contains(&"node-0".to_string()));
        assert!(loc.node_ids.contains(&"node-1".to_string()));
    }

    #[tokio::test]
    async fn record_and_query_refs() {
        let (_s, _d, engine) = make_engine().await;
        let chunk_id = fake_chunk_id();

        engine.record_chunk_ref(&chunk_id, "vol-1").unwrap();
        engine.record_chunk_ref(&chunk_id, "vol-2").unwrap();

        let cr = engine.location_store.get_ref(&chunk_id).unwrap().unwrap();
        assert_eq!(cr.volume_ids.len(), 2);
    }

    #[tokio::test]
    async fn reconcile_healthy_no_actions() {
        let (_s, _d, engine) = make_engine().await;
        let chunk_id = fake_chunk_id();

        // Set up: chunk on 1 node, policy wants 1 replica.
        engine.record_chunk_location(&chunk_id, "node-0").unwrap();
        engine.record_chunk_ref(&chunk_id, "vol-1").unwrap();
        engine
            .set_policy(VolumePolicy::new("vol-1".to_string(), 1))
            .await;

        let action_count = engine.reconcile().await.unwrap();
        assert_eq!(action_count, 0, "healthy chunk should need 0 actions");
    }

    #[tokio::test]
    async fn reconcile_uses_evaluator_for_under_replicated() {
        // Verify the engine feeds evaluator correctly for under-replicated
        // chunks.  We use a single-node topology so CRUSH cannot find new
        // targets and no gRPC connections are attempted.
        let store_dir = tempfile::tempdir().unwrap();
        let db_dir = tempfile::tempdir().unwrap();

        let file_store =
            FileChunkStore::new(store_dir.path().to_path_buf(), 64 * 1024 * 1024).unwrap();
        let location_store =
            Arc::new(ChunkLocationStore::open(db_dir.path().join("locations.redb")).unwrap());

        // Single-node topology: only node-0.
        let mut topo = ClusterMap::new(1);
        topo.add_node(Node {
            id: "node-0".to_string(),
            address: "10.0.0.1".to_string(),
            port: 9100,
            backends: vec![Backend {
                id: "bdev-0".to_string(),
                node_id: "node-0".to_string(),
                capacity_bytes: 1_000_000_000_000,
                used_bytes: 0,
                weight: 100,
                backend_type: BackendType::Bdev,
            }],
            status: NodeStatus::Online,
        });

        let engine = PolicyEngine::new(
            "node-0".to_string(),
            location_store,
            Arc::new(file_store),
            topo,
        );

        let chunk_id = fake_chunk_id();

        // Chunk on node-0 only, policy wants 3.  CRUSH can only return
        // node-0 (already holds the chunk) so no Replicate actions are
        // generated — but the chunk IS under-replicated.  Verify 0 actions
        // because there are no candidates, confirming the engine correctly
        // evaluates but respects placement constraints.
        engine.record_chunk_location(&chunk_id, "node-0").unwrap();
        engine.record_chunk_ref(&chunk_id, "vol-1").unwrap();
        engine
            .set_policy(VolumePolicy::new("vol-1".to_string(), 3))
            .await;

        let action_count = engine.reconcile().await.unwrap();
        assert_eq!(
            action_count, 0,
            "no actions when CRUSH cannot find new target nodes"
        );
    }

    #[tokio::test]
    async fn reconcile_over_replicated_local_removal() {
        let (store_dir, _d, engine) = make_engine().await;
        let chunk_id = fake_chunk_id();

        // Seed the chunk in local store so local delete succeeds.
        let data = make_chunk_data(b"over-replicated test data");
        let chunk_path = store_dir
            .path()
            .join("chunks")
            .join(&chunk_id[..2])
            .join(&chunk_id[2..4]);
        std::fs::create_dir_all(&chunk_path).unwrap();
        std::fs::write(chunk_path.join(&chunk_id), &data).unwrap();

        // Put chunk on 2 nodes with node-0 LAST so the evaluator's
        // over-replicated logic removes it (removes from end of list).
        // node-0 is local, so remove_replica goes through local store.
        engine
            .location_store
            .add_node_to_chunk(&chunk_id, "node-1")
            .unwrap();
        engine
            .location_store
            .add_node_to_chunk(&chunk_id, "node-0")
            .unwrap();
        engine.record_chunk_ref(&chunk_id, "vol-1").unwrap();
        engine
            .set_policy(VolumePolicy::new("vol-1".to_string(), 1))
            .await;

        let action_count = engine.reconcile().await.unwrap();
        assert_eq!(
            action_count, 1,
            "over-replicated by 1 should generate 1 action"
        );

        // Verify the chunk was removed from the local store.
        let loc = engine
            .location_store
            .get_location(&chunk_id)
            .unwrap()
            .unwrap();
        assert_eq!(loc.node_ids.len(), 1);
        assert_eq!(loc.node_ids[0], "node-1");
    }

    #[tokio::test]
    async fn reconcile_no_refs_no_actions() {
        let (_s, _d, engine) = make_engine().await;
        let chunk_id = fake_chunk_id();

        // Chunk exists in the location store but has no volume ref — the
        // evaluator skips chunks without refs.
        engine.record_chunk_location(&chunk_id, "node-0").unwrap();
        engine
            .set_policy(VolumePolicy::new("vol-1".to_string(), 3))
            .await;

        let action_count = engine.reconcile().await.unwrap();
        assert_eq!(action_count, 0);
    }

    #[tokio::test]
    async fn update_topology() {
        let (_s, _d, engine) = make_engine().await;

        let mut new_topo = ClusterMap::new(100);
        new_topo.add_node(Node {
            id: "node-new".to_string(),
            address: "192.168.1.1".to_string(),
            port: 9200,
            backends: vec![],
            status: NodeStatus::Online,
        });

        engine.update_topology(new_topo).await;

        // Verify the topology was replaced by checking generation.
        let topo = engine.topology.read().await;
        // new(100) + add_node increments to 101
        assert_eq!(topo.generation(), 101);
        assert!(topo.get_node("node-new").is_some());
        // Old nodes should be gone.
        assert!(topo.get_node("node-0").is_none());
    }

    #[tokio::test]
    async fn node_id_accessor() {
        let (_s, _d, engine) = make_engine().await;
        assert_eq!(engine.node_id(), "node-0");
    }
}
