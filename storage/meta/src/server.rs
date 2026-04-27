//! gRPC server: implements `MetaService` over redb-backed `Store`.
//!
//! The server is transport-agnostic: bind it to either a `UnixListener`
//! (default in production) or a `TcpListener` (default for tests).

use std::time::{SystemTime, UNIX_EPOCH};

use tonic::{Request, Response, Status};
use tracing::{debug, info};

use crate::crush;
use crate::proto::meta_service_server::{MetaService, MetaServiceServer};
use crate::proto::{self, *};
use crate::store::Store;
use crate::topology::PoolTopology;
use crate::types::{self, ChunkPlacement, Disk, Pool, ProtectionSpec, Task, TaskPayload, Volume};

/// gRPC service implementation.
pub struct MetaServer {
    store: Store,
}

impl MetaServer {
    pub fn new(store: Store) -> Self {
        Self { store }
    }

    pub fn into_service(self) -> MetaServiceServer<Self> {
        MetaServiceServer::new(self)
    }
}

fn now_secs() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or_default()
}

fn map_err<E: std::fmt::Display>(e: E) -> Status {
    Status::internal(e.to_string())
}

/// Build a `PoolTopology` for placement out of the disks belonging to a pool
/// and currently present + ready.
pub fn pool_topology(store: &Store, pool_uuid: &str) -> anyhow::Result<PoolTopology> {
    let mut topo = PoolTopology::new(pool_uuid.to_string());
    for d in store.list_disks_for_pool(pool_uuid)? {
        if d.present && (d.state == "ready" || d.state == "claiming") {
            // Weight by GiB.
            let gib = (d.size_bytes / (1024 * 1024 * 1024)).max(1);
            topo.push(d.uuid.clone(), gib);
        }
    }
    Ok(topo)
}

#[tonic::async_trait]
impl MetaService for MetaServer {
    // --- Pools ----------------------------------------------------------

    async fn put_pool(
        &self,
        req: Request<PutPoolRequest>,
    ) -> Result<Response<PutPoolResponse>, Status> {
        let pool = req
            .into_inner()
            .pool
            .ok_or_else(|| Status::invalid_argument("pool missing"))?;
        if pool.uuid.is_empty() {
            return Err(Status::invalid_argument("pool.uuid empty"));
        }
        let mut p: Pool = pool.into();
        p.generation = p.generation.max(1);
        self.store.put_pool(&p).map_err(map_err)?;
        debug!(uuid = %p.uuid, "pool put");
        Ok(Response::new(PutPoolResponse {
            pool: Some(p.into()),
        }))
    }

    async fn get_pool(
        &self,
        req: Request<GetPoolRequest>,
    ) -> Result<Response<GetPoolResponse>, Status> {
        let uuid = req.into_inner().uuid;
        let p = self
            .store
            .get_pool(&uuid)
            .map_err(map_err)?
            .ok_or_else(|| Status::not_found(format!("pool {uuid} not found")))?;
        Ok(Response::new(GetPoolResponse {
            pool: Some(p.into()),
        }))
    }

    async fn list_pools(
        &self,
        _req: Request<ListPoolsRequest>,
    ) -> Result<Response<ListPoolsResponse>, Status> {
        let pools = self
            .store
            .list_pools()
            .map_err(map_err)?
            .into_iter()
            .map(Into::into)
            .collect();
        Ok(Response::new(ListPoolsResponse { pools }))
    }

    async fn delete_pool(
        &self,
        req: Request<DeletePoolRequest>,
    ) -> Result<Response<DeletePoolResponse>, Status> {
        self.store
            .delete_pool(&req.into_inner().uuid)
            .map_err(map_err)?;
        Ok(Response::new(DeletePoolResponse {}))
    }

    // --- Disks ----------------------------------------------------------

    async fn put_disk(
        &self,
        req: Request<PutDiskRequest>,
    ) -> Result<Response<PutDiskResponse>, Status> {
        let disk = req
            .into_inner()
            .disk
            .ok_or_else(|| Status::invalid_argument("disk missing"))?;
        if disk.uuid.is_empty() {
            return Err(Status::invalid_argument("disk.uuid empty"));
        }
        let mut d: Disk = disk.into();
        d.generation = d.generation.max(1);
        self.store.put_disk(&d).map_err(map_err)?;
        Ok(Response::new(PutDiskResponse {
            disk: Some(d.into()),
        }))
    }

    async fn get_disk(
        &self,
        req: Request<GetDiskRequest>,
    ) -> Result<Response<GetDiskResponse>, Status> {
        let uuid = req.into_inner().uuid;
        let d = self
            .store
            .get_disk(&uuid)
            .map_err(map_err)?
            .ok_or_else(|| Status::not_found(format!("disk {uuid} not found")))?;
        Ok(Response::new(GetDiskResponse {
            disk: Some(d.into()),
        }))
    }

    async fn list_disks(
        &self,
        req: Request<ListDisksRequest>,
    ) -> Result<Response<ListDisksResponse>, Status> {
        let pool = req.into_inner().pool_uuid;
        let disks = if pool.is_empty() {
            self.store.list_disks().map_err(map_err)?
        } else {
            self.store.list_disks_for_pool(&pool).map_err(map_err)?
        };
        Ok(Response::new(ListDisksResponse {
            disks: disks.into_iter().map(Into::into).collect(),
        }))
    }

    async fn delete_disk(
        &self,
        req: Request<DeleteDiskRequest>,
    ) -> Result<Response<DeleteDiskResponse>, Status> {
        self.store
            .delete_disk(&req.into_inner().uuid)
            .map_err(map_err)?;
        Ok(Response::new(DeleteDiskResponse {}))
    }

    // --- Volumes --------------------------------------------------------

    async fn create_volume(
        &self,
        req: Request<CreateVolumeRequest>,
    ) -> Result<Response<CreateVolumeResponse>, Status> {
        let r = req.into_inner();
        if r.uuid.is_empty() {
            return Err(Status::invalid_argument("uuid empty"));
        }
        if r.size_bytes == 0 {
            return Err(Status::invalid_argument("size_bytes must be > 0"));
        }
        let pool = self
            .store
            .get_pool(&r.pool_uuid)
            .map_err(map_err)?
            .ok_or_else(|| Status::not_found(format!("pool {} not found", r.pool_uuid)))?;
        let prot: ProtectionSpec = r.protection.map(Into::into).unwrap_or(ProtectionSpec {
            replication_factor: pool.replication_factor,
        });
        let rf = if prot.replication_factor == 0 {
            pool.replication_factor
        } else {
            prot.replication_factor
        };
        let topo = pool_topology(&self.store, &r.pool_uuid).map_err(map_err)?;
        if (topo.len() as u32) < rf {
            return Err(Status::failed_precondition(format!(
                "pool {} has {} eligible disks, need {}",
                r.pool_uuid,
                topo.len(),
                rf
            )));
        }
        let chunk_count = crate::chunk_count_for(r.size_bytes);
        let mut placements = Vec::with_capacity(chunk_count as usize);
        for i in 0..chunk_count {
            let key = crush::chunk_key(&r.uuid, i as u32);
            let disks = crush::select(&key, rf as usize, &topo)
                .map_err(|e| Status::failed_precondition(format!("CRUSH placement failed: {e}")))?;
            placements.push(ChunkPlacement {
                index: i as u32,
                chunk_id: String::new(),
                disk_uuids: disks,
            });
        }

        let v = Volume {
            uuid: r.uuid.clone(),
            name: r.name,
            pool_uuid: r.pool_uuid,
            size_bytes: r.size_bytes,
            protection: ProtectionSpec {
                replication_factor: rf,
            },
            chunk_size_bytes: crate::CHUNK_SIZE_BYTES,
            chunk_count,
            generation: 1,
        };
        self.store.put_volume(&v).map_err(map_err)?;
        self.store
            .put_chunk_map(&v.uuid, &placements)
            .map_err(map_err)?;
        info!(uuid = %v.uuid, chunks = chunk_count, "volume created");

        let chunk_map = types::ChunkMap {
            volume_uuid: v.uuid.clone(),
            chunks: placements,
        };
        Ok(Response::new(CreateVolumeResponse {
            volume: Some(v.into()),
            chunk_map: Some(chunk_map.into()),
        }))
    }

    async fn get_volume(
        &self,
        req: Request<GetVolumeRequest>,
    ) -> Result<Response<GetVolumeResponse>, Status> {
        let uuid = req.into_inner().uuid;
        let v = self
            .store
            .get_volume(&uuid)
            .map_err(map_err)?
            .ok_or_else(|| Status::not_found(format!("volume {uuid} not found")))?;
        Ok(Response::new(GetVolumeResponse {
            volume: Some(v.into()),
        }))
    }

    async fn list_volumes(
        &self,
        req: Request<ListVolumesRequest>,
    ) -> Result<Response<ListVolumesResponse>, Status> {
        let pool = req.into_inner().pool_uuid;
        let mut vols = self.store.list_volumes().map_err(map_err)?;
        if !pool.is_empty() {
            vols.retain(|v| v.pool_uuid == pool);
        }
        Ok(Response::new(ListVolumesResponse {
            volumes: vols.into_iter().map(Into::into).collect(),
        }))
    }

    async fn delete_volume(
        &self,
        req: Request<DeleteVolumeRequest>,
    ) -> Result<Response<DeleteVolumeResponse>, Status> {
        self.store
            .delete_volume(&req.into_inner().uuid)
            .map_err(map_err)?;
        Ok(Response::new(DeleteVolumeResponse {}))
    }

    async fn get_chunk_map(
        &self,
        req: Request<GetChunkMapRequest>,
    ) -> Result<Response<GetChunkMapResponse>, Status> {
        let cm = self
            .store
            .get_chunk_map(&req.into_inner().volume_uuid)
            .map_err(map_err)?;
        Ok(Response::new(GetChunkMapResponse {
            chunk_map: Some(cm.into()),
        }))
    }

    // --- Claim / release -----------------------------------------------

    async fn claim_disk(
        &self,
        req: Request<ClaimDiskRequest>,
    ) -> Result<Response<ClaimDiskResponse>, Status> {
        let r = req.into_inner();
        let mut d = self
            .store
            .get_disk(&r.disk_uuid)
            .map_err(map_err)?
            .ok_or_else(|| Status::not_found(format!("disk {} not found", r.disk_uuid)))?;
        d.pool_uuid = r.pool_uuid.clone();
        d.state = "claiming".into();
        d.generation += 1;
        self.store.put_disk(&d).map_err(map_err)?;
        let task = Task {
            id: uuid::Uuid::new_v4().to_string(),
            created_unix_secs: now_secs(),
            payload: TaskPayload::ClaimDisk {
                disk_uuid: d.uuid.clone(),
                pool_uuid: r.pool_uuid,
            },
        };
        self.store.put_task(&task).map_err(map_err)?;
        Ok(Response::new(ClaimDiskResponse {
            disk: Some(d.into()),
        }))
    }

    async fn release_disk(
        &self,
        req: Request<ReleaseDiskRequest>,
    ) -> Result<Response<ReleaseDiskResponse>, Status> {
        let r = req.into_inner();
        let mut d = self
            .store
            .get_disk(&r.disk_uuid)
            .map_err(map_err)?
            .ok_or_else(|| Status::not_found(format!("disk {} not found", r.disk_uuid)))?;
        d.pool_uuid = String::new();
        d.state = "unclaimed".into();
        d.generation += 1;
        self.store.put_disk(&d).map_err(map_err)?;
        let task = Task {
            id: uuid::Uuid::new_v4().to_string(),
            created_unix_secs: now_secs(),
            payload: TaskPayload::ReleaseDisk {
                disk_uuid: d.uuid.clone(),
            },
        };
        self.store.put_task(&task).map_err(map_err)?;
        Ok(Response::new(ReleaseDiskResponse {
            disk: Some(d.into()),
        }))
    }

    // --- Tasks ---------------------------------------------------------

    async fn poll_tasks(
        &self,
        req: Request<PollTasksRequest>,
    ) -> Result<Response<PollTasksResponse>, Status> {
        let max = req.into_inner().max as usize;
        let mut tasks = self.store.list_tasks().map_err(map_err)?;
        if max > 0 && tasks.len() > max {
            tasks.truncate(max);
        }
        let proto_tasks = tasks.iter().map(|t| t.to_proto()).collect();
        Ok(Response::new(PollTasksResponse { tasks: proto_tasks }))
    }

    async fn ack_task(
        &self,
        req: Request<AckTaskRequest>,
    ) -> Result<Response<AckTaskResponse>, Status> {
        let r = req.into_inner();
        // Always remove on ack: a failure ack means the data daemon gave up.
        // Re-emission, if needed, comes from the policy loop.
        if !r.success {
            tracing::warn!(task = %r.task_id, error = %r.error, "task acked with failure");
        }
        self.store.delete_task(&r.task_id).map_err(map_err)?;
        Ok(Response::new(AckTaskResponse {}))
    }

    async fn heartbeat(
        &self,
        _req: Request<HeartbeatRequest>,
    ) -> Result<Response<HeartbeatResponse>, Status> {
        Ok(Response::new(HeartbeatResponse {
            server_unix_secs: now_secs(),
        }))
    }
}

// Suppress unused-warn for proto module re-exports used only as type aliases.
#[allow(dead_code)]
fn _proto_uses() {
    let _: Option<proto::Pool> = None;
}
