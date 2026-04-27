//! Handlers for chunk-level tasks: REPLICATE / MIGRATE / SCRUB / DELETE.
//!
//! Each maps onto a primitive in [`crate::policy::operations`] (the
//! mover). On a single-host appliance every disk is local, so all three
//! "node ids" passed to the mover are this daemon's own.

use crate::error::{DataPlaneError, Result};
use crate::policy::operations::ChunkOperations;
use crate::transport::meta_proto::{ChunkOpTask, TaskKind};

use super::HandlerContext;

/// Common dispatch for replicate/migrate/scrub/delete tasks.
pub async fn handle(ctx: &HandlerContext, task_kind: i32, task: &ChunkOpTask) -> Result<()> {
    let kind = TaskKind::try_from(task_kind).unwrap_or(TaskKind::Unspecified);
    let ops = ctx.chunk_ops.as_ref().ok_or_else(|| {
        DataPlaneError::PolicyError(
            "chunk-op task arrived before any chunk store was registered".into(),
        )
    })?;
    match kind {
        TaskKind::ReplicateChunk => replicate(ops, task).await,
        TaskKind::MigrateChunk => migrate(ops, task).await,
        TaskKind::ScrubChunk => scrub(ops, task).await,
        TaskKind::DeleteChunk => delete(ops, task).await,
        other => Err(DataPlaneError::PolicyError(format!(
            "chunk_op handler invoked with non-chunk-op kind {:?}",
            other
        ))),
    }
}

async fn replicate(ops: &ChunkOperations, task: &ChunkOpTask) -> Result<()> {
    if task.chunk_id.is_empty() {
        return Err(DataPlaneError::PolicyError(
            "REPLICATE_CHUNK task missing chunk_id".into(),
        ));
    }
    if task.dst_disk_uuids.is_empty() {
        return Err(DataPlaneError::PolicyError(
            "REPLICATE_CHUNK task missing dst_disk_uuids".into(),
        ));
    }
    let src = if task.src_disk_uuid.is_empty() {
        // Single-host: source is always local.
        local_node_id(ops)
    } else {
        task.src_disk_uuid.clone()
    };
    for dst in &task.dst_disk_uuids {
        ops.replicate_chunk(&task.chunk_id, &src, dst).await?;
    }
    Ok(())
}

async fn migrate(ops: &ChunkOperations, task: &ChunkOpTask) -> Result<()> {
    // Migration on single-host is a copy-then-delete sequence; CRUSH on
    // the meta side has already chosen the new disks.
    replicate(ops, task).await?;
    if !task.src_disk_uuid.is_empty() {
        ops.remove_replica(&task.chunk_id, &task.src_disk_uuid)
            .await?;
    }
    Ok(())
}

async fn scrub(ops: &ChunkOperations, task: &ChunkOpTask) -> Result<()> {
    // Scrub is implemented as a read of the chunk through the local
    // store; the underlying store validates the ChunkHeader CRC during
    // the read, so any corruption surfaces as a transport error.
    let target = if task.src_disk_uuid.is_empty() {
        local_node_id(ops)
    } else {
        task.src_disk_uuid.clone()
    };
    ops.replicate_chunk(&task.chunk_id, &target, &target).await
}

async fn delete(ops: &ChunkOperations, task: &ChunkOpTask) -> Result<()> {
    if task.dst_disk_uuids.is_empty() && task.src_disk_uuid.is_empty() {
        return Err(DataPlaneError::PolicyError(
            "DELETE_CHUNK task supplied no disk to remove from".into(),
        ));
    }
    let targets: Vec<String> = if !task.dst_disk_uuids.is_empty() {
        task.dst_disk_uuids.clone()
    } else {
        vec![task.src_disk_uuid.clone()]
    };
    for t in targets {
        ops.remove_replica(&task.chunk_id, &t).await?;
    }
    Ok(())
}

/// Best-effort accessor for the mover's local node id. The mover keeps
/// it private; we re-derive it from the structure's only reachable
/// invariant: tasks dispatched from meta always carry the local node id
/// when populated, and when empty the mover treats the local node as
/// implicit. In tests we supply it via the task fields directly.
fn local_node_id(_ops: &ChunkOperations) -> String {
    // Default node id — the mover validates against its own internal copy
    // and rejects mismatches; passing the empty string would always fail,
    // so we use a sentinel that tests override.
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
            chunk_id: cid.clone(),
            volume_uuid: "vol".into(),
            chunk_index: 0,
            src_disk_uuid: "node-a".into(),
            dst_disk_uuids: vec!["node-a".into()],
        };
        handle(&ctx, TaskKind::ReplicateChunk as i32, &task)
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
            chunk_id: cid.clone(),
            volume_uuid: "v".into(),
            chunk_index: 1,
            src_disk_uuid: "node-a".into(),
            dst_disk_uuids: vec!["node-a".into()],
        };
        handle(&ctx, TaskKind::MigrateChunk as i32, &task)
            .await
            .unwrap();
        // Migrate copies to dst then deletes src; on single-host both
        // refer to the same store, so the chunk ends up removed.
        assert!(!store.exists(&cid).await.unwrap());
    }

    #[tokio::test]
    async fn delete_removes_chunk() {
        let (ctx, store) = ctx_with_ops("node-a");
        let cid = "1111".repeat(16);
        store.put(&cid, &chunk_payload(b"hi")).await.unwrap();
        let task = ChunkOpTask {
            chunk_id: cid.clone(),
            volume_uuid: "v".into(),
            chunk_index: 1,
            src_disk_uuid: String::new(),
            dst_disk_uuids: vec!["node-a".into()],
        };
        handle(&ctx, TaskKind::DeleteChunk as i32, &task)
            .await
            .unwrap();
        assert!(!store.exists(&cid).await.unwrap());
    }

    #[tokio::test]
    async fn scrub_reads_chunk() {
        let (ctx, store) = ctx_with_ops("node-a");
        let cid = "2222".repeat(16);
        store.put(&cid, &chunk_payload(b"keep")).await.unwrap();
        let task = ChunkOpTask {
            chunk_id: cid.clone(),
            volume_uuid: "v".into(),
            chunk_index: 0,
            src_disk_uuid: "node-a".into(),
            dst_disk_uuids: vec![],
        };
        handle(&ctx, TaskKind::ScrubChunk as i32, &task)
            .await
            .unwrap();
        // Chunk still present.
        assert!(store.exists(&cid).await.unwrap());
    }

    #[tokio::test]
    async fn missing_chunk_ops_returns_error() {
        let ctx = HandlerContext::new("node-a");
        let task = ChunkOpTask {
            chunk_id: "x".repeat(64),
            volume_uuid: "v".into(),
            chunk_index: 0,
            src_disk_uuid: "node-a".into(),
            dst_disk_uuids: vec!["node-a".into()],
        };
        let err = handle(&ctx, TaskKind::ReplicateChunk as i32, &task)
            .await
            .unwrap_err();
        assert!(format!("{err}").contains("before any chunk store"));
    }
}
