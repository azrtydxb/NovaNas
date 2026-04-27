//! Handlers for tasks dispatched by `novanas-meta` via
//! `MetaService::PollTasks`.
//!
//! Each [`TaskKind`](crate::transport::meta_proto::TaskKind) maps to a
//! handler module here; the [`TaskRunner`](crate::task_runner::TaskRunner)
//! routes incoming `Task` messages to [`handle_task`] and reports the
//! outcome back via `MetaService::AckTask`.

pub mod chunk_op;
pub mod claim_disk;
pub mod release_disk;

use std::sync::Arc;

use crate::error::{DataPlaneError, Result};
use crate::policy::operations::ChunkOperations;
use crate::transport::meta_proto::{Task, TaskKind};

/// Shared services every task handler can consume.
pub struct HandlerContext {
    /// Identifier the daemon registered itself with at startup.
    pub node_id: String,
    /// Path to the sysfs root used by [`disk_discovery`](crate::disk_discovery)
    /// and [`device`](crate::device). Tests override this; production
    /// uses `/sys`.
    pub sysfs_root: std::path::PathBuf,
    /// NVMe driver-binding manager.
    pub device_manager: crate::device::Manager,
    /// Chunk-op mover used for replicate/migrate/scrub/delete tasks.
    /// `None` until a chunk store has been registered (no claimed disks
    /// yet) — handlers requiring it will return `PolicyError`.
    pub chunk_ops: Option<Arc<ChunkOperations>>,
}

impl HandlerContext {
    /// Build a context with the production defaults.
    pub fn new(node_id: impl Into<String>) -> Self {
        Self {
            node_id: node_id.into(),
            sysfs_root: std::path::PathBuf::from("/sys"),
            device_manager: crate::device::Manager::default(),
            chunk_ops: None,
        }
    }

    /// Override the sysfs root (also reconfigures the device manager).
    pub fn with_sysfs_root(mut self, root: impl Into<std::path::PathBuf>) -> Self {
        let p = root.into();
        self.device_manager = crate::device::Manager::with_sysfs_root(p.clone());
        self.sysfs_root = p;
        self
    }

    /// Attach the mover; until set, chunk-op tasks fail.
    pub fn with_chunk_ops(mut self, ops: Arc<ChunkOperations>) -> Self {
        self.chunk_ops = Some(ops);
        self
    }
}

/// Dispatch `task` to the matching handler. Returns `Ok(())` on success,
/// or an error the runner records on the AckTask call.
pub async fn handle_task(ctx: &HandlerContext, task: &Task) -> Result<()> {
    match TaskKind::try_from(task.kind).unwrap_or(TaskKind::Unspecified) {
        TaskKind::ClaimDisk => {
            let claim = task.claim_disk.as_ref().ok_or_else(|| {
                DataPlaneError::PolicyError("CLAIM_DISK task missing claim_disk payload".into())
            })?;
            claim_disk::handle(ctx, claim).await
        }
        TaskKind::ReleaseDisk => {
            let release = task.release_disk.as_ref().ok_or_else(|| {
                DataPlaneError::PolicyError("RELEASE_DISK task missing release_disk payload".into())
            })?;
            release_disk::handle(ctx, release).await
        }
        TaskKind::ReplicateChunk
        | TaskKind::MigrateChunk
        | TaskKind::ScrubChunk
        | TaskKind::DeleteChunk => {
            let op = task.chunk_op.as_ref().ok_or_else(|| {
                DataPlaneError::PolicyError("chunk-op task missing chunk_op payload".into())
            })?;
            chunk_op::handle(ctx, task.kind, op).await
        }
        TaskKind::Unspecified => Err(DataPlaneError::PolicyError(format!(
            "task {} has unspecified kind",
            task.id
        ))),
    }
}
