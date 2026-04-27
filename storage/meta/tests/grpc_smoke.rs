//! End-to-end gRPC smoke test: spin up a real `MetaServer` on a temp UDS,
//! call every RPC at least once via a tonic client.

use std::path::PathBuf;
use std::time::Duration;

use novanas_meta::proto::meta_service_client::MetaServiceClient;
use novanas_meta::proto::*;
use novanas_meta::server::MetaServer;
use novanas_meta::store::Store;
use tokio::net::{UnixListener, UnixStream};
use tokio_stream::wrappers::UnixListenerStream;
use tonic::transport::{Endpoint, Server, Uri};
use tower::service_fn;

async fn spawn_server() -> (MetaServiceClient<tonic::transport::Channel>, PathBuf) {
    let dir = tempfile::tempdir().unwrap();
    let socket = dir.path().join("meta.sock");
    let store_path = dir.path().join("meta.redb");
    let store = Store::open(&store_path).unwrap();
    let svc = MetaServer::new(store).into_service();

    // Leak temp dir for test lifetime.
    let leaked: &'static tempfile::TempDir = Box::leak(Box::new(dir));
    let _ = leaked.path();

    let listener = UnixListener::bind(&socket).unwrap();
    let stream = UnixListenerStream::new(listener);
    tokio::spawn(async move {
        Server::builder()
            .add_service(svc)
            .serve_with_incoming(stream)
            .await
            .unwrap();
    });
    // Give server a moment to start accepting.
    tokio::time::sleep(Duration::from_millis(50)).await;

    let socket_for_connect = socket.clone();
    let channel = Endpoint::try_from("http://[::]:50051")
        .unwrap()
        .connect_with_connector(service_fn(move |_: Uri| {
            let p = socket_for_connect.clone();
            async move {
                let s = UnixStream::connect(p).await?;
                Ok::<_, std::io::Error>(hyper_util::rt::TokioIo::new(s))
            }
        }))
        .await
        .unwrap();
    (MetaServiceClient::new(channel), socket)
}

#[tokio::test]
async fn end_to_end_rpcs() {
    let (mut client, _sock) = spawn_server().await;

    // Heartbeat
    let hb = client
        .heartbeat(HeartbeatRequest {
            client_id: "test".into(),
        })
        .await
        .unwrap();
    assert!(hb.into_inner().server_unix_secs > 0);

    // PutPool / GetPool / ListPools
    client
        .put_pool(PutPoolRequest {
            pool: Some(Pool {
                uuid: "p1".into(),
                name: "pool1".into(),
                replication_factor: 2,
                tier: "fast".into(),
                generation: 0,
            }),
        })
        .await
        .unwrap();
    let got = client
        .get_pool(GetPoolRequest { uuid: "p1".into() })
        .await
        .unwrap()
        .into_inner()
        .pool
        .unwrap();
    assert_eq!(got.uuid, "p1");
    let pools = client
        .list_pools(ListPoolsRequest {})
        .await
        .unwrap()
        .into_inner()
        .pools;
    assert_eq!(pools.len(), 1);

    // PutDisk x3
    for i in 0..3 {
        client
            .put_disk(PutDiskRequest {
                disk: Some(Disk {
                    uuid: format!("d{i}"),
                    node: "local".into(),
                    device_path: format!("/dev/x{i}"),
                    size_bytes: 10 * 1024 * 1024 * 1024,
                    pool_uuid: "p1".into(),
                    state: "ready".into(),
                    tier: "fast".into(),
                    present: true,
                    generation: 0,
                }),
            })
            .await
            .unwrap();
    }
    let disks = client
        .list_disks(ListDisksRequest {
            pool_uuid: "p1".into(),
        })
        .await
        .unwrap()
        .into_inner()
        .disks;
    assert_eq!(disks.len(), 3);
    let _ = client
        .get_disk(GetDiskRequest { uuid: "d0".into() })
        .await
        .unwrap();

    // ClaimDisk + ReleaseDisk
    client
        .claim_disk(ClaimDiskRequest {
            disk_uuid: "d0".into(),
            pool_uuid: "p1".into(),
        })
        .await
        .unwrap();
    client
        .release_disk(ReleaseDiskRequest {
            disk_uuid: "d0".into(),
        })
        .await
        .unwrap();
    // Re-claim so volume creation has 3 eligible disks.
    client
        .put_disk(PutDiskRequest {
            disk: Some(Disk {
                uuid: "d0".into(),
                node: "local".into(),
                device_path: "/dev/x0".into(),
                size_bytes: 10 * 1024 * 1024 * 1024,
                pool_uuid: "p1".into(),
                state: "ready".into(),
                tier: "fast".into(),
                present: true,
                generation: 0,
            }),
        })
        .await
        .unwrap();

    // CreateVolume
    let cv = client
        .create_volume(CreateVolumeRequest {
            uuid: "v1".into(),
            name: "vol".into(),
            pool_uuid: "p1".into(),
            size_bytes: 16 * 1024 * 1024,
            protection: Some(ProtectionSpec {
                replication_factor: 2,
            }),
        })
        .await
        .unwrap()
        .into_inner();
    assert_eq!(cv.volume.as_ref().unwrap().chunk_count, 4);
    assert_eq!(cv.chunk_map.as_ref().unwrap().chunks.len(), 4);

    let cm = client
        .get_chunk_map(GetChunkMapRequest {
            volume_uuid: "v1".into(),
        })
        .await
        .unwrap()
        .into_inner()
        .chunk_map
        .unwrap();
    assert_eq!(cm.chunks.len(), 4);
    for c in &cm.chunks {
        assert_eq!(c.disk_uuids.len(), 2);
    }

    // ListVolumes / GetVolume
    let vols = client
        .list_volumes(ListVolumesRequest {
            pool_uuid: "p1".into(),
        })
        .await
        .unwrap()
        .into_inner()
        .volumes;
    assert_eq!(vols.len(), 1);
    let _ = client
        .get_volume(GetVolumeRequest { uuid: "v1".into() })
        .await
        .unwrap();

    // PollTasks (we should have at least the claim+release tasks from above).
    let tasks = client
        .poll_tasks(PollTasksRequest { max: 100 })
        .await
        .unwrap()
        .into_inner()
        .tasks;
    assert!(!tasks.is_empty());
    let first_id = tasks[0].id.clone();
    client
        .ack_task(AckTaskRequest {
            task_id: first_id.clone(),
            success: true,
            error: String::new(),
        })
        .await
        .unwrap();
    let after = client
        .poll_tasks(PollTasksRequest { max: 100 })
        .await
        .unwrap()
        .into_inner()
        .tasks;
    assert!(after.iter().all(|t| t.id != first_id));

    // DeleteVolume / DeleteDisk / DeletePool
    client
        .delete_volume(DeleteVolumeRequest { uuid: "v1".into() })
        .await
        .unwrap();
    client
        .delete_disk(DeleteDiskRequest { uuid: "d0".into() })
        .await
        .unwrap();
    client
        .delete_pool(DeletePoolRequest { uuid: "p1".into() })
        .await
        .unwrap();
}

#[tokio::test]
async fn create_volume_errors_when_too_few_disks() {
    let (mut client, _) = spawn_server().await;
    client
        .put_pool(PutPoolRequest {
            pool: Some(Pool {
                uuid: "p".into(),
                name: "p".into(),
                replication_factor: 3,
                tier: "fast".into(),
                generation: 0,
            }),
        })
        .await
        .unwrap();
    // Only one disk, replication 3 -> error.
    client
        .put_disk(PutDiskRequest {
            disk: Some(Disk {
                uuid: "only".into(),
                node: "local".into(),
                device_path: "/dev/x".into(),
                size_bytes: 10 * 1024 * 1024 * 1024,
                pool_uuid: "p".into(),
                state: "ready".into(),
                tier: "fast".into(),
                present: true,
                generation: 0,
            }),
        })
        .await
        .unwrap();
    let err = client
        .create_volume(CreateVolumeRequest {
            uuid: "v".into(),
            name: "v".into(),
            pool_uuid: "p".into(),
            size_bytes: 4 * 1024 * 1024,
            protection: Some(ProtectionSpec {
                replication_factor: 3,
            }),
        })
        .await
        .unwrap_err();
    assert_eq!(err.code(), tonic::Code::FailedPrecondition);
}
