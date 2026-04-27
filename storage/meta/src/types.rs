//! Rust value types persisted in redb, plus conversions to/from the prost
//! generated wire types.

use serde::{Deserialize, Serialize};

use crate::proto;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct Pool {
    pub uuid: String,
    pub name: String,
    pub replication_factor: u32,
    pub tier: String,
    pub generation: u64,
}

impl From<Pool> for proto::Pool {
    fn from(p: Pool) -> Self {
        Self {
            uuid: p.uuid,
            name: p.name,
            replication_factor: p.replication_factor,
            tier: p.tier,
            generation: p.generation,
        }
    }
}

impl From<proto::Pool> for Pool {
    fn from(p: proto::Pool) -> Self {
        Self {
            uuid: p.uuid,
            name: p.name,
            replication_factor: if p.replication_factor == 0 {
                2
            } else {
                p.replication_factor
            },
            tier: p.tier,
            generation: p.generation,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct Disk {
    pub uuid: String,
    pub node: String,
    pub device_path: String,
    pub size_bytes: u64,
    pub pool_uuid: String,
    pub state: String,
    pub tier: String,
    pub present: bool,
    pub generation: u64,
}

impl From<Disk> for proto::Disk {
    fn from(d: Disk) -> Self {
        Self {
            uuid: d.uuid,
            node: d.node,
            device_path: d.device_path,
            size_bytes: d.size_bytes,
            pool_uuid: d.pool_uuid,
            state: d.state,
            tier: d.tier,
            present: d.present,
            generation: d.generation,
        }
    }
}

impl From<proto::Disk> for Disk {
    fn from(d: proto::Disk) -> Self {
        Self {
            uuid: d.uuid,
            node: d.node,
            device_path: d.device_path,
            size_bytes: d.size_bytes,
            pool_uuid: d.pool_uuid,
            state: d.state,
            tier: d.tier,
            present: d.present,
            generation: d.generation,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ProtectionSpec {
    pub replication_factor: u32,
}

impl From<ProtectionSpec> for proto::ProtectionSpec {
    fn from(p: ProtectionSpec) -> Self {
        Self {
            replication_factor: p.replication_factor,
        }
    }
}

impl From<proto::ProtectionSpec> for ProtectionSpec {
    fn from(p: proto::ProtectionSpec) -> Self {
        Self {
            replication_factor: p.replication_factor,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct Volume {
    pub uuid: String,
    pub name: String,
    pub pool_uuid: String,
    pub size_bytes: u64,
    pub protection: ProtectionSpec,
    pub chunk_size_bytes: u64,
    pub chunk_count: u64,
    pub generation: u64,
}

impl From<Volume> for proto::Volume {
    fn from(v: Volume) -> Self {
        Self {
            uuid: v.uuid,
            name: v.name,
            pool_uuid: v.pool_uuid,
            size_bytes: v.size_bytes,
            protection: Some(v.protection.into()),
            chunk_size_bytes: v.chunk_size_bytes,
            chunk_count: v.chunk_count,
            generation: v.generation,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
pub struct ChunkPlacement {
    pub index: u32,
    pub chunk_id: String,
    pub disk_uuids: Vec<String>,
}

impl From<ChunkPlacement> for proto::ChunkPlacement {
    fn from(c: ChunkPlacement) -> Self {
        Self {
            index: c.index,
            chunk_id: c.chunk_id,
            disk_uuids: c.disk_uuids,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
pub struct ChunkMap {
    pub volume_uuid: String,
    pub chunks: Vec<ChunkPlacement>,
}

impl From<ChunkMap> for proto::ChunkMap {
    fn from(m: ChunkMap) -> Self {
        Self {
            volume_uuid: m.volume_uuid,
            chunks: m.chunks.into_iter().map(Into::into).collect(),
        }
    }
}

// ---------------------------------------------------------------------------
// Tasks
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub enum TaskPayload {
    ClaimDisk {
        disk_uuid: String,
        pool_uuid: String,
    },
    ReleaseDisk {
        disk_uuid: String,
    },
    ReplicateChunk {
        volume_uuid: String,
        chunk_index: u32,
        source_disk_uuids: Vec<String>,
        target_disk_uuids: Vec<String>,
    },
    // Future: TierMigrateChunk { ... }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct Task {
    pub id: String,
    pub created_unix_secs: u64,
    pub payload: TaskPayload,
}

impl Task {
    pub fn to_proto(&self) -> proto::Task {
        let (kind, payload) = match &self.payload {
            TaskPayload::ClaimDisk {
                disk_uuid,
                pool_uuid,
            } => (
                proto::TaskKind::TaskClaimDisk,
                Some(proto::task::Payload::ClaimDisk(proto::ClaimDiskTask {
                    disk_uuid: disk_uuid.clone(),
                    pool_uuid: pool_uuid.clone(),
                })),
            ),
            TaskPayload::ReleaseDisk { disk_uuid } => (
                proto::TaskKind::TaskReleaseDisk,
                Some(proto::task::Payload::ReleaseDisk(proto::ReleaseDiskTask {
                    disk_uuid: disk_uuid.clone(),
                })),
            ),
            TaskPayload::ReplicateChunk {
                volume_uuid,
                chunk_index,
                source_disk_uuids,
                target_disk_uuids,
            } => (
                proto::TaskKind::TaskReplicateChunk,
                Some(proto::task::Payload::ChunkOp(proto::ChunkOpTask {
                    volume_uuid: volume_uuid.clone(),
                    chunk_index: *chunk_index,
                    source_disk_uuids: source_disk_uuids.clone(),
                    target_disk_uuids: target_disk_uuids.clone(),
                    // chunk_id is filled by callers when meta has already
                    // allocated a content-addressed identifier for this
                    // chunk; left empty for unallocated chunks.
                    chunk_id: String::new(),
                })),
            ),
        };
        proto::Task {
            id: self.id.clone(),
            kind: kind as i32,
            created_unix_secs: self.created_unix_secs,
            payload,
        }
    }
}
