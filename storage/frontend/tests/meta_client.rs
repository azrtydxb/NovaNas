//! Integration test: stand up an in-process MetaService gRPC server,
//! point a `UdsMetaClient` at its UDS, and round-trip GetChunkMap +
//! GetVolume + Heartbeat.

use std::path::PathBuf;
use std::sync::Arc;

use tokio::net::UnixListener;
use tokio_stream::wrappers::UnixListenerStream;
use tonic::{transport::Server, Request, Response, Status};

use novanas_frontend::meta_client::{MetaClient, UdsMetaClient};
use novanas_frontend::proto::meta::meta_service_server::{MetaService, MetaServiceServer};
use novanas_frontend::proto::meta::{
    AckTaskRequest, ChunkMapEntry, ChunkMapSlice, ClaimDiskRequest, CreateVolumeRequest,
    DaemonKind, Disk, DiskList, DiskRef, GetChunkMapRequest, HeartbeatRequest, HeartbeatResponse,
    ListDisksRequest, PollTasksRequest, Pool, PoolList, PoolRef, TaskBatch, Volume, VolumeList,
    VolumeRef,
};

#[derive(Default)]
struct FakeMeta;

#[tonic::async_trait]
impl MetaService for FakeMeta {
    async fn put_pool(&self, _r: Request<Pool>) -> Result<Response<()>, Status> {
        Ok(Response::new(()))
    }
    async fn get_pool(&self, _r: Request<PoolRef>) -> Result<Response<Pool>, Status> {
        Err(Status::unimplemented("get_pool"))
    }
    async fn list_pools(&self, _r: Request<()>) -> Result<Response<PoolList>, Status> {
        Ok(Response::new(PoolList { items: vec![] }))
    }
    async fn delete_pool(&self, _r: Request<PoolRef>) -> Result<Response<()>, Status> {
        Ok(Response::new(()))
    }
    async fn put_disk(&self, _r: Request<Disk>) -> Result<Response<()>, Status> {
        Ok(Response::new(()))
    }
    async fn get_disk(&self, _r: Request<DiskRef>) -> Result<Response<Disk>, Status> {
        Err(Status::unimplemented("get_disk"))
    }
    async fn list_disks(
        &self,
        _r: Request<ListDisksRequest>,
    ) -> Result<Response<DiskList>, Status> {
        Ok(Response::new(DiskList { items: vec![] }))
    }
    async fn delete_disk(&self, _r: Request<DiskRef>) -> Result<Response<()>, Status> {
        Ok(Response::new(()))
    }
    async fn create_volume(
        &self,
        _r: Request<CreateVolumeRequest>,
    ) -> Result<Response<Volume>, Status> {
        Err(Status::unimplemented("create_volume"))
    }
    async fn get_volume(&self, req: Request<VolumeRef>) -> Result<Response<Volume>, Status> {
        Ok(Response::new(Volume {
            name: req.into_inner().name,
            uuid: "uuid".into(),
            pool_name: "pool".into(),
            size_bytes: 4096 * 1024,
            chunk_size_bytes: 4 * 1024 * 1024,
            chunk_count: 1,
            protection: None,
            phase: "Ready".into(),
            created_at: None,
        }))
    }
    async fn list_volumes(&self, _r: Request<()>) -> Result<Response<VolumeList>, Status> {
        Ok(Response::new(VolumeList {
            items: vec![Volume {
                name: "v1".into(),
                uuid: "u1".into(),
                pool_name: "p".into(),
                size_bytes: 0,
                chunk_size_bytes: 0,
                chunk_count: 0,
                protection: None,
                phase: "Ready".into(),
                created_at: None,
            }],
        }))
    }
    async fn delete_volume(&self, _r: Request<VolumeRef>) -> Result<Response<()>, Status> {
        Ok(Response::new(()))
    }
    async fn get_chunk_map(
        &self,
        req: Request<GetChunkMapRequest>,
    ) -> Result<Response<ChunkMapSlice>, Status> {
        let r = req.into_inner();
        let entries = (r.chunk_index_lo..r.chunk_index_hi)
            .map(|i| ChunkMapEntry {
                chunk_index: i,
                chunk_id: format!("cid-{}", i),
                disk_wwns: vec!["wwn-x".into()],
            })
            .collect();
        Ok(Response::new(ChunkMapSlice { entries }))
    }
    async fn claim_disk(&self, _r: Request<ClaimDiskRequest>) -> Result<Response<Disk>, Status> {
        Err(Status::unimplemented("claim_disk"))
    }
    async fn release_disk(&self, _r: Request<DiskRef>) -> Result<Response<()>, Status> {
        Ok(Response::new(()))
    }
    async fn poll_tasks(
        &self,
        _r: Request<PollTasksRequest>,
    ) -> Result<Response<TaskBatch>, Status> {
        Ok(Response::new(TaskBatch { items: vec![] }))
    }
    async fn ack_task(&self, _r: Request<AckTaskRequest>) -> Result<Response<()>, Status> {
        Ok(Response::new(()))
    }
    async fn heartbeat(
        &self,
        req: Request<HeartbeatRequest>,
    ) -> Result<Response<HeartbeatResponse>, Status> {
        let r = req.into_inner();
        Ok(Response::new(HeartbeatResponse {
            desired_crush_digest: vec![],
            pending_task_count: r.disks.len() as u32,
        }))
    }
}

async fn start_server(socket: PathBuf) {
    let _ = std::fs::remove_file(&socket);
    let listener = UnixListener::bind(&socket).expect("bind UDS");
    let stream = UnixListenerStream::new(listener);
    tokio::spawn(async move {
        let _ = Server::builder()
            .add_service(MetaServiceServer::new(FakeMeta))
            .serve_with_incoming(stream)
            .await;
    });
    // Give the server a moment to start accepting.
    tokio::time::sleep(std::time::Duration::from_millis(50)).await;
}

#[tokio::test]
async fn round_trip_get_chunk_map_and_volume() {
    let dir = tempfile::tempdir().unwrap();
    let socket = dir.path().join("meta.sock");
    start_server(socket.clone()).await;

    let client = Arc::new(UdsMetaClient::connect(&socket).await.unwrap()) as Arc<dyn MetaClient>;
    let slice = client.get_chunk_map("vol", 0, 4).await.unwrap();
    let ids: Vec<String> = slice.entries.iter().map(|e| e.chunk_id.clone()).collect();
    assert_eq!(
        ids,
        vec![
            "cid-0".to_string(),
            "cid-1".to_string(),
            "cid-2".to_string(),
            "cid-3".to_string(),
        ]
    );
    let v = client.get_volume("alpha").await.unwrap();
    assert_eq!(v.name, "alpha");
    let resp = client
        .heartbeat(HeartbeatRequest {
            node_id: "node-A".into(),
            kind: DaemonKind::Frontend as i32,
            version: "0.1.0".into(),
            disks: vec![],
        })
        .await
        .unwrap();
    assert_eq!(resp.pending_task_count, 0);
}
