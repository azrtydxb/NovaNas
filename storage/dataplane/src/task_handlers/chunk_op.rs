//! Handlers for chunk-level tasks.
//!
//! Wrapper-style `TaskKind` exposes only `REPLICATE_CHUNK` and
//! `TIER_MIGRATE_CHUNK` for chunk movement; SCRUB/DELETE are not modelled
//! on the wire and are not emitted by meta in the architecture-v2
//! single-host design. Each surviving kind maps onto a primitive in
//! [`crate::policy::operations`] (the mover). On a single-host appliance
//! every disk is local, so the "node ids" passed to the mover are this
//! daemon's own.

use crate::error::{DataPlaneError, Result};
use crate::policy::operations::ChunkOperations;
use crate::transport::meta_proto::{ChunkOpTask, TaskKind};

use super::HandlerContext;

/// Dispatch a chunk-op task. `task_kind` distinguishes plain replication
/// from a tier-migration (replicate then drop the source).
pub async fn handle(ctx: &HandlerContext, task_kind: i32, task: &ChunkOpTask) -> Result<()> {
    let kind = TaskKind::try_from(task_kind).unwrap_or(TaskKind::TaskUnknown);
    let ops = ctx.chunk_ops.as_ref().ok_or_else(|| {
        DataPlaneError::PolicyError(
            "chunk-op task arrived before any chunk store was registered".into(),
        )
    })?;
    match kind {
        TaskKind::TaskReplicateChunk => replicate(ops, task).await,
        TaskKind::TaskTierMigrateChunk => migrate(ops, task).await,
        other => Err(DataPlaneError::PolicyError(format!(
            "chunk_op handler invoked with non-chunk-op kind {:?}",
            other
        ))),
    }
}

fn require_chunk_id(task: &ChunkOpTask) -> Result<&str> {
    if task.chunk_id.is_empty() {
        return Err(DataPlaneError::PolicyError(format!(
            "chunk-op task for volume {} chunk {} missing chunk_id",
            task.volume_uuid, task.chunk_index
        )));
    }
    Ok(task.chunk_id.as_str())
}

fn primary_source(task: &ChunkOpTask) -> String {
    task.source_disk_uuids
        .first()
        .cloned()
        .unwrap_or_else(local_node_id)
}

async fn replicate(ops: &ChunkOperations, task: &ChunkOpTask) -> Result<()> {
    let chunk_id = require_chunk_id(task)?.to_string();
    if task.target_disk_uuids.is_empty() {
        return Err(DataPlaneError::PolicyError(
            "REPLICATE_CHUNK task missing target_disk_uuids".into(),
        ));
    }
    let src = primary_source(task);
    for dst in &task.target_disk_uuids {
        ops.replicate_chunk(&chunk_id, &src, dst).await?;
    }
    Ok(())
}

async fn migrate(ops: &ChunkOperations, task: &ChunkOpTask) -> Result<()> {
    // Migration on single-host is a copy-then-delete sequence; CRUSH on
    // the meta side has already chosen the new disks.
    let chunk_id = require_chunk_id(task)?.to_string();
    replicate(ops, task).await?;
    for src in &task.source_disk_uuids {
        ops.remove_replica(&chunk_id, src).await?;
    }
    Ok(())
}

/// Default node id used when the task did not name a source disk. The
/// mover validates against its own internal copy and rejects mismatches;
/// tests always populate `source_disk_uuids` explicitly.
fn local_node_id() -> String {
    "self".to_string()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::backend::chunk_store::{ChunkHeader, ChunkStore, ChunkStoreStats};
    use async_trait::async_trait;
    use std::collections::HashMap;
    use std::sync::Arc;
    use std::sync::Mutex;

    struct MemStore {
        inner: Mutex<HashMap<String, Vec<u8>>>,
    }
    impl MemStore {
        fn new() -> Self {
            Self {
                inner: Mutex::new(HashMap::new()),
            }
        }
    }
    #[async_trait]
    impl ChunkStore for MemStore {
        async fn put(&self, chunk_id: &str, data: &[u8]) -> Result<()> {
            self.inner
                .lock()
                .unwrap()
                .insert(chunk_id.to_string(), data.to_vec());
            Ok(())
        }
        async fn get(&self, chunk_id: &str) -> Result<Vec<u8>> {
            self.inner
                .lock()
                .unwrap()
                .get(chunk_id)
                .cloned()
                .ok_or_else(|| DataPlaneError::ChunkEngineError(format!("not found: {chunk_id}")))
        }
        async fn delete(&self, chunk_id: &str) -> Result<()> {
            self.inner.lock().unwrap().remove(chunk_id).ok_or_else(|| {
                DataPlaneError::ChunkEngineError(format!("not found: {chunk_id}"))
            })?;
            Ok(())
        }
        async fn exists(&self, chunk_id: &str) -> Result<bool> {
            Ok(self.inner.lock().unwrap().contains_key(chunk_id))
        }
        async fn stats(&self) -> Result<ChunkStoreStats> {
            Ok(ChunkStoreStats {
                backend_name: "mem".into(),
                total_bytes: 0,
                used_bytes: 0,
                data_bytes: 0,
                chunk_count: 0,
            })
        }
    }

    fn ctx_with_ops(node_id: &str) -> (HandlerContext, Arc<MemStore>) {
        let store = Arc::new(MemStore::new());
        let ops = Arc::new(ChunkOperations::new(node_id.into(), store.clone()));
        let ctx = HandlerContext::new(node_id).with_chunk_ops(ops);
        (ctx, store)
    }

    fn chunk_payload(payload: &[u8]) -> Vec<u8> {
        let header = ChunkHeader {
            magic: *b"NVAC",
            version: 1,
            flags: 0,
            checksum: crc32c::crc32c(payload),
            data_len: payload.len() as u32,
            _reserved: [0; 2],
        };
        let mut out = Vec::with_capacity(ChunkHeader::SIZE + payload.len());
        out.extend_from_slice(&header.to_bytes());
        out.extend_from_slice(payload);
        out
    }

    #[tokio::test]
    async fn replicate_with_local_dst_succeeds() {
        let (ctx, store) = ctx_with_ops("node-a");
        let cid = "deadbeef".repeat(8);
        store.put(&cid, &chunk_payload(b"hi")).await.unwrap();
        let task = ChunkOpTask {
            volume_uuid: "vol".into(),
            chunk_index: 0,
            source_disk_uuids: vec!["node-a".into()],
            target_disk_uuids: vec!["node-a".into()],
            chunk_id: cid.clone(),
        };
        handle(&ctx, TaskKind::TaskReplicateChunk as i32, &task)
            .await
            .unwrap();
        assert!(store.exists(&cid).await.unwrap());
    }

    #[tokio::test]
    async fn migrate_copies_then_deletes_when_src_set() {
        let (ctx, store) = ctx_with_ops("node-a");
        let cid = "abcd".repeat(16);
        store.put(&cid, &chunk_payload(b"hi")).await.unwrap();
        let task = ChunkOpTask {
            volume_uuid: "v".into(),
            chunk_index: 1,
            source_disk_uuids: vec!["node-a".into()],
            target_disk_uuids: vec!["node-a".into()],
            chunk_id: cid.clone(),
        };
        handle(&ctx, TaskKind::TaskTierMigrateChunk as i32, &task)
            .await
            .unwrap();
        // Migrate copies to dst then deletes src; on single-host both
        // refer to the same store, so the chunk ends up removed.
        assert!(!store.exists(&cid).await.unwrap());
    }

    #[tokio::test]
    async fn missing_chunk_ops_returns_error() {
        let ctx = HandlerContext::new("node-a");
        let task = ChunkOpTask {
            volume_uuid: "v".into(),
            chunk_index: 0,
            source_disk_uuids: vec!["node-a".into()],
            target_disk_uuids: vec!["node-a".into()],
            chunk_id: "x".repeat(64),
        };
        let err = handle(&ctx, TaskKind::TaskReplicateChunk as i32, &task)
            .await
            .unwrap_err();
        assert!(format!("{err}").contains("before any chunk store"));
    }

    #[tokio::test]
    async fn replicate_without_chunk_id_errors() {
        let (ctx, _store) = ctx_with_ops("node-a");
        let task = ChunkOpTask {
            volume_uuid: "v".into(),
            chunk_index: 0,
            source_disk_uuids: vec!["node-a".into()],
            target_disk_uuids: vec!["node-a".into()],
            chunk_id: String::new(),
        };
        let err = handle(&ctx, TaskKind::TaskReplicateChunk as i32, &task)
            .await
            .unwrap_err();
        assert!(format!("{err}").contains("missing chunk_id"));
    }
}
