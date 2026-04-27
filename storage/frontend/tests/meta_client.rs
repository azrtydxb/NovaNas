//! Integration test: stand up an in-process MetaService gRPC server using
//! the wrapper-style canonical contract, point a `UdsMetaClient` at its
//! UDS, and round-trip GetChunkMap + GetVolume + Heartbeat.

use std::path::PathBuf;
use std::sync::Arc;

use tokio::net::UnixListener;
use tokio_stream::wrappers::UnixListenerStream;
use tonic::{transport::Server, Request, Response, Status};

use novanas_frontend::meta_client::{MetaClient, UdsMetaClient};
use novanas_frontend::proto::meta::meta_service_server::{MetaService, MetaServiceServer};
use novanas_frontend::proto::meta::{
    AckTaskRequest, AckTaskResponse, ChunkMap, ChunkPlacement, ClaimDiskRequest, ClaimDiskResponse,
    CreateVolumeRequest, CreateVolumeResponse, DeleteDiskRequest, DeleteDiskResponse,
    DeletePoolRequest, DeletePoolResponse, DeleteVolumeRequest, DeleteVolumeResponse,
    GetChunkMapRequest, GetChunkMapResponse, GetDiskRequest, GetDiskResponse, GetPoolRequest,
    GetPoolResponse, GetVolumeRequest, GetVolumeResponse, HeartbeatRequest, HeartbeatResponse,
    ListDisksRequest, ListDisksResponse, ListPoolsRequest, ListPoolsResponse, ListVolumesRequest,
    ListVolumesResponse, PollTasksRequest, PollTasksResponse, PutDiskRequest, PutDiskResponse,
    PutPoolRequest, PutPoolResponse, ReleaseDiskRequest, ReleaseDiskResponse, Volume,
};

#[derive(Default)]
struct FakeMeta;

fn sample_volume(name: &str, uuid: &str) -> Volume {
    Volume {
        name: name.to_string(),
        uuid: uuid.to_string(),
        pool_uuid: "pool".into(),
        size_bytes: 4096 * 1024,
        chunk_size_bytes: 4 * 1024 * 1024,
        chunk_count: 1,
        protection: None,
        generation: 1,
    }
}

#[tonic::async_trait]
impl MetaService for FakeMeta {
    async fn put_pool(
        &self,
        r: Request<PutPoolRequest>,
    ) -> Result<Response<PutPoolResponse>, Status> {
        Ok(Response::new(PutPoolResponse {
            pool: r.into_inner().pool,
        }))
    }
    async fn get_pool(
        &self,
        _r: Request<GetPoolRequest>,
    ) -> Result<Response<GetPoolResponse>, Status> {
        Err(Status::unimplemented("get_pool"))
    }
    async fn list_pools(
        &self,
        _r: Request<ListPoolsRequest>,
    ) -> Result<Response<ListPoolsResponse>, Status> {
        Ok(Response::new(ListPoolsResponse { pools: vec![] }))
    }
    async fn delete_pool(
        &self,
        _r: Request<DeletePoolRequest>,
    ) -> Result<Response<DeletePoolResponse>, Status> {
        Ok(Response::new(DeletePoolResponse {}))
    }
    async fn put_disk(
        &self,
        r: Request<PutDiskRequest>,
    ) -> Result<Response<PutDiskResponse>, Status> {
        Ok(Response::new(PutDiskResponse {
            disk: r.into_inner().disk,
        }))
    }
    async fn get_disk(
        &self,
        _r: Request<GetDiskRequest>,
    ) -> Result<Response<GetDiskResponse>, Status> {
        Err(Status::unimplemented("get_disk"))
    }
    async fn list_disks(
        &self,
        _r: Request<ListDisksRequest>,
    ) -> Result<Response<ListDisksResponse>, Status> {
        Ok(Response::new(ListDisksResponse { disks: vec![] }))
    }
    async fn delete_disk(
        &self,
        _r: Request<DeleteDiskRequest>,
    ) -> Result<Response<DeleteDiskResponse>, Status> {
        Ok(Response::new(DeleteDiskResponse {}))
    }
    async fn create_volume(
        &self,
        _r: Request<CreateVolumeRequest>,
    ) -> Result<Response<CreateVolumeResponse>, Status> {
        Err(Status::unimplemented("create_volume"))
    }
    async fn get_volume(
        &self,
        req: Request<GetVolumeRequest>,
    ) -> Result<Response<GetVolumeResponse>, Status> {
        let uuid = req.into_inner().uuid;
        Ok(Response::new(GetVolumeResponse {
            volume: Some(sample_volume("alpha", &uuid)),
        }))
    }
    async fn list_volumes(
        &self,
        _r: Request<ListVolumesRequest>,
    ) -> Result<Response<ListVolumesResponse>, Status> {
        Ok(Response::new(ListVolumesResponse {
            volumes: vec![
                sample_volume("alpha", "u-alpha"),
                sample_volume("vol", "u-vol"),
            ],
        }))
    }
    async fn delete_volume(
        &self,
        _r: Request<DeleteVolumeRequest>,
    ) -> Result<Response<DeleteVolumeResponse>, Status> {
        Ok(Response::new(DeleteVolumeResponse {}))
    }
    async fn get_chunk_map(
        &self,
        req: Request<GetChunkMapRequest>,
    ) -> Result<Response<GetChunkMapResponse>, Status> {
        let volume_uuid = req.into_inner().volume_uuid;
        // Always return a 16-chunk map; the client slices it down.
        let chunks = (0..16u32)
            .map(|i| ChunkPlacement {
                index: i,
                chunk_id: format!("cid-{}", i),
                disk_uuids: vec!["disk-x".into()],
            })
            .collect();
        Ok(Response::new(GetChunkMapResponse {
            chunk_map: Some(ChunkMap {
                volume_uuid,
                chunks,
            }),
        }))
    }
    async fn claim_disk(
        &self,
        _r: Request<ClaimDiskRequest>,
    ) -> Result<Response<ClaimDiskResponse>, Status> {
        Err(Status::unimplemented("claim_disk"))
    }
    async fn release_disk(
        &self,
        _r: Request<ReleaseDiskRequest>,
    ) -> Result<Response<ReleaseDiskResponse>, Status> {
        Err(Status::unimplemented("release_disk"))
    }
    async fn poll_tasks(
        &self,
        _r: Request<PollTasksRequest>,
    ) -> Result<Response<PollTasksResponse>, Status> {
        Ok(Response::new(PollTasksResponse { tasks: vec![] }))
    }
    async fn ack_task(
        &self,
        _r: Request<AckTaskRequest>,
    ) -> Result<Response<AckTaskResponse>, Status> {
        Ok(Response::new(AckTaskResponse {}))
    }
    async fn heartbeat(
        &self,
        _req: Request<HeartbeatRequest>,
    ) -> Result<Response<HeartbeatResponse>, Status> {
        Ok(Response::new(HeartbeatResponse {
            server_unix_secs: 42,
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
    let placements = client.get_chunk_map("alpha", 0, 4).await.unwrap();
    let ids: Vec<String> = placements.iter().map(|e| e.chunk_id.clone()).collect();
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
            client_id: "frontend-test".into(),
        })
        .await
        .unwrap();
    assert_eq!(resp.server_unix_secs, 42);
}
