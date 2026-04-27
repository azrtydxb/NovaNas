//! Timer-driven policy checker.
//!
//! Walks every volume's chunk map and ensures replication-factor compliance.
//! When a chunk has fewer healthy copies than its volume's protection spec
//! requires (or copies on degraded disks), enqueue a `REPLICATE_CHUNK` task
//! pointing at fresh CRUSH-selected target disks.
//!
//! Tier migration is a future hook; the loop body has a clearly-named branch
//! for it but does not emit tasks in this PR.

use std::collections::{HashMap, HashSet};
use std::time::Duration;

use anyhow::Result;
use tracing::{debug, info, warn};

use crate::crush;
use crate::server::pool_topology;
use crate::store::Store;
use crate::types::{ChunkPlacement, Disk, Task, TaskPayload};

#[derive(Debug, Clone)]
pub struct PolicyConfig {
    pub interval: Duration,
}

impl Default for PolicyConfig {
    fn default() -> Self {
        Self {
            interval: Duration::from_secs(30),
        }
    }
}

pub struct PolicyChecker {
    cfg: PolicyConfig,
    store: Store,
}

impl PolicyChecker {
    pub fn new(cfg: PolicyConfig, store: Store) -> Self {
        Self { cfg, store }
    }

    pub async fn run(self) -> Result<()> {
        let mut iv = tokio::time::interval(self.cfg.interval);
        iv.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
        loop {
            iv.tick().await;
            if let Err(e) = self.tick() {
                warn!(error = %e, "policy tick failed");
            }
        }
    }

    /// One reconciliation pass. Public so tests can drive it.
    pub fn tick(&self) -> Result<()> {
        let disks: HashMap<String, Disk> = self
            .store
            .list_disks()?
            .into_iter()
            .map(|d| (d.uuid.clone(), d))
            .collect();
        let volumes = self.store.list_volumes()?;
        for v in volumes {
            let cm = self.store.get_chunk_map(&v.uuid)?;
            let topo = pool_topology(&self.store, &v.pool_uuid)?;
            let healthy_set: HashSet<String> =
                topo.disks.iter().map(|d| d.disk_uuid.clone()).collect();
            for chunk in &cm.chunks {
                let healthy_copies: Vec<&String> = chunk
                    .disk_uuids
                    .iter()
                    .filter(|d| {
                        healthy_set.contains(*d)
                            && disks.get(*d).map(|x| x.present).unwrap_or(false)
                    })
                    .collect();
                let required = v.protection.replication_factor as usize;
                if healthy_copies.len() < required {
                    self.enqueue_replicate(&v.uuid, chunk, &topo, required)?;
                }
                // Future hook: tier migration.
                self.maybe_tier_migrate(&v, chunk);
            }
        }
        Ok(())
    }

    fn enqueue_replicate(
        &self,
        volume_uuid: &str,
        chunk: &ChunkPlacement,
        topo: &crate::topology::PoolTopology,
        required: usize,
    ) -> Result<()> {
        // Pick fresh placement; CRUSH is deterministic so the source set is
        // either a subset of the new picks (already-placed copies) or
        // disjoint (everything has moved). The data daemon does the actual
        // copy + record-update.
        let key = crush::chunk_key(volume_uuid, chunk.index);
        let target_count = required.min(topo.len());
        if target_count == 0 {
            warn!(volume = %volume_uuid, chunk = chunk.index, "no eligible disks for replication");
            return Ok(());
        }
        let targets = match crush::select(&key, target_count, topo) {
            Ok(v) => v,
            Err(e) => {
                warn!(volume = %volume_uuid, error = %e, "CRUSH replicate selection failed");
                return Ok(());
            }
        };
        let already = self.store.task_exists_for(|t| match &t.payload {
            TaskPayload::ReplicateChunk {
                volume_uuid: vu,
                chunk_index,
                ..
            } => vu == volume_uuid && *chunk_index == chunk.index,
            _ => false,
        })?;
        if already {
            debug!(volume = %volume_uuid, chunk = chunk.index, "replicate task already pending");
            return Ok(());
        }
        let task = Task {
            id: uuid::Uuid::new_v4().to_string(),
            created_unix_secs: now_secs(),
            payload: TaskPayload::ReplicateChunk {
                volume_uuid: volume_uuid.to_string(),
                chunk_index: chunk.index,
                source_disk_uuids: chunk.disk_uuids.clone(),
                target_disk_uuids: targets,
            },
        };
        self.store.put_task(&task)?;
        info!(volume = %volume_uuid, chunk = chunk.index, "replicate task enqueued");
        Ok(())
    }

    /// Future hook: tier migration. Not emitted in this PR; leaving the
    /// branch named so the next agent can fill it in without grepping for
    /// TODOs.
    #[inline]
    fn maybe_tier_migrate(&self, _v: &crate::types::Volume, _chunk: &ChunkPlacement) {
        // Intentionally empty. Tier migration tasks are out of scope for the
        // architecture-v2 meta initial PR.
    }
}

fn now_secs() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or_default()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::types::{Pool, ProtectionSpec, Volume};

    fn temp_store() -> Store {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("s.redb");
        let leaked: &'static tempfile::TempDir = Box::leak(Box::new(dir));
        let _ = leaked.path();
        Store::open(path).unwrap()
    }

    fn mk_disk(uuid: &str, pool: &str, present: bool, state: &str) -> Disk {
        Disk {
            uuid: uuid.into(),
            node: "n".into(),
            device_path: "/dev/x".into(),
            size_bytes: 10 * 1024 * 1024 * 1024,
            pool_uuid: pool.into(),
            state: state.into(),
            tier: "fast".into(),
            present,
            generation: 1,
        }
    }

    #[test]
    fn policy_emits_replicate_when_under_replicated() {
        let s = temp_store();
        let pool = Pool {
            uuid: "p".into(),
            name: "p".into(),
            replication_factor: 2,
            tier: "fast".into(),
            generation: 1,
        };
        s.put_pool(&pool).unwrap();
        for i in 0..3 {
            s.put_disk(&mk_disk(&format!("d{i}"), "p", true, "ready"))
                .unwrap();
        }
        let vol = Volume {
            uuid: "v".into(),
            name: "v".into(),
            pool_uuid: "p".into(),
            size_bytes: 4 * 1024 * 1024,
            protection: ProtectionSpec {
                replication_factor: 2,
            },
            chunk_size_bytes: 4 * 1024 * 1024,
            chunk_count: 1,
            generation: 1,
        };
        s.put_volume(&vol).unwrap();
        // Place chunk on only ONE disk -> under-replicated.
        s.put_chunk_map(
            "v",
            &[ChunkPlacement {
                index: 0,
                chunk_id: String::new(),
                disk_uuids: vec!["d0".into()],
            }],
        )
        .unwrap();

        let p = PolicyChecker::new(PolicyConfig::default(), s.clone());
        p.tick().unwrap();
        let tasks = s.list_tasks().unwrap();
        assert_eq!(tasks.len(), 1);
        match &tasks[0].payload {
            TaskPayload::ReplicateChunk {
                volume_uuid,
                chunk_index,
                ..
            } => {
                assert_eq!(volume_uuid, "v");
                assert_eq!(*chunk_index, 0);
            }
            _ => panic!("expected replicate task"),
        }
    }

    #[test]
    fn policy_does_not_double_emit() {
        let s = temp_store();
        let pool = Pool {
            uuid: "p".into(),
            name: "p".into(),
            replication_factor: 2,
            tier: "fast".into(),
            generation: 1,
        };
        s.put_pool(&pool).unwrap();
        for i in 0..3 {
            s.put_disk(&mk_disk(&format!("d{i}"), "p", true, "ready"))
                .unwrap();
        }
        let vol = Volume {
            uuid: "v".into(),
            name: "v".into(),
            pool_uuid: "p".into(),
            size_bytes: 4 * 1024 * 1024,
            protection: ProtectionSpec {
                replication_factor: 2,
            },
            chunk_size_bytes: 4 * 1024 * 1024,
            chunk_count: 1,
            generation: 1,
        };
        s.put_volume(&vol).unwrap();
        s.put_chunk_map(
            "v",
            &[ChunkPlacement {
                index: 0,
                chunk_id: String::new(),
                disk_uuids: vec!["d0".into()],
            }],
        )
        .unwrap();
        let p = PolicyChecker::new(PolicyConfig::default(), s.clone());
        p.tick().unwrap();
        p.tick().unwrap();
        assert_eq!(s.list_tasks().unwrap().len(), 1);
    }

    #[test]
    fn policy_compliant_emits_nothing() {
        let s = temp_store();
        let pool = Pool {
            uuid: "p".into(),
            name: "p".into(),
            replication_factor: 2,
            tier: "fast".into(),
            generation: 1,
        };
        s.put_pool(&pool).unwrap();
        for i in 0..3 {
            s.put_disk(&mk_disk(&format!("d{i}"), "p", true, "ready"))
                .unwrap();
        }
        let vol = Volume {
            uuid: "v".into(),
            name: "v".into(),
            pool_uuid: "p".into(),
            size_bytes: 4 * 1024 * 1024,
            protection: ProtectionSpec {
                replication_factor: 2,
            },
            chunk_size_bytes: 4 * 1024 * 1024,
            chunk_count: 1,
            generation: 1,
        };
        s.put_volume(&vol).unwrap();
        s.put_chunk_map(
            "v",
            &[ChunkPlacement {
                index: 0,
                chunk_id: String::new(),
                disk_uuids: vec!["d0".into(), "d1".into()],
            }],
        )
        .unwrap();
        let p = PolicyChecker::new(PolicyConfig::default(), s.clone());
        p.tick().unwrap();
        assert_eq!(s.list_tasks().unwrap().len(), 0);
    }
}
