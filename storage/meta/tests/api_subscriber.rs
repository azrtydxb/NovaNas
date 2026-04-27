//! Hand-rolled fake `novanas-api` HTTP server. Drives `ApiSubscriber::tick`
//! and asserts the resulting local store state.

use std::sync::Arc;
use std::time::Duration;

use novanas_meta::api_client::{ApiSubscriber, ApiSubscriberConfig};
use novanas_meta::store::Store;
use novanas_meta::types::TaskPayload;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;
use tokio::sync::Mutex;

#[derive(Default)]
struct FakeApi {
    pools: String,
    disks: String,
    volumes: String,
}

async fn run_fake_api(state: Arc<Mutex<FakeApi>>) -> u16 {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let port = listener.local_addr().unwrap().port();
    tokio::spawn(async move {
        loop {
            let (mut sock, _) = listener.accept().await.unwrap();
            let state = state.clone();
            tokio::spawn(async move {
                let mut buf = [0u8; 8192];
                let n = sock.read(&mut buf).await.unwrap_or(0);
                if n == 0 {
                    return;
                }
                let req = String::from_utf8_lossy(&buf[..n]);
                let body = {
                    let s = state.lock().await;
                    if req.contains("/api/v1/pools") {
                        s.pools.clone()
                    } else if req.contains("/api/v1/disks") {
                        s.disks.clone()
                    } else if req.contains("/api/v1/block-volumes") {
                        s.volumes.clone()
                    } else {
                        "[]".to_string()
                    }
                };
                let resp = format!(
                    "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    body.len(),
                    body
                );
                let _ = sock.write_all(resp.as_bytes()).await;
                let _ = sock.shutdown().await;
            });
        }
    });
    port
}

fn temp_store() -> Store {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("s.redb");
    let leaked: &'static tempfile::TempDir = Box::leak(Box::new(dir));
    let _ = leaked.path();
    Store::open(path).unwrap()
}

#[tokio::test]
async fn subscriber_reconciles_full_view() {
    let state = Arc::new(Mutex::new(FakeApi::default()));
    let port = run_fake_api(state.clone()).await;

    {
        let mut s = state.lock().await;
        s.pools = r#"[{"uuid":"p1","name":"p1","replicationFactor":2,"tier":"fast"}]"#.into();
        s.disks = r#"[
            {"uuid":"d0","sizeBytes":10737418240,"poolUuid":"p1","state":"ready","tier":"fast","present":true,"devicePath":"/dev/x0"},
            {"uuid":"d1","sizeBytes":10737418240,"poolUuid":"p1","state":"ready","tier":"fast","present":true,"devicePath":"/dev/x1"},
            {"uuid":"d2","sizeBytes":10737418240,"poolUuid":"p1","state":"ready","tier":"fast","present":true,"devicePath":"/dev/x2"}
        ]"#.into();
        s.volumes = r#"[{"uuid":"v1","name":"vol1","poolUuid":"p1","sizeBytes":16777216,"protection":{"replicationFactor":2}}]"#.into();
    }

    let store = temp_store();
    let cfg = ApiSubscriberConfig {
        base_url: format!("http://127.0.0.1:{port}"),
        poll_interval: Duration::from_secs(60),
        node_name: "test".into(),
    };
    let sub = ApiSubscriber::new(cfg, store.clone()).unwrap();
    sub.tick().await.unwrap();

    assert_eq!(store.list_pools().unwrap().len(), 1);
    assert_eq!(store.list_disks().unwrap().len(), 3);
    assert_eq!(store.list_volumes().unwrap().len(), 1);

    let cm = store.get_chunk_map("v1").unwrap();
    assert_eq!(cm.chunks.len(), 4); // 16 MiB / 4 MiB
    for c in &cm.chunks {
        assert_eq!(c.disk_uuids.len(), 2);
    }

    // Three claim tasks should have been emitted (one per claimed disk).
    let tasks = store.list_tasks().unwrap();
    let claim_count = tasks
        .iter()
        .filter(|t| matches!(t.payload, TaskPayload::ClaimDisk { .. }))
        .count();
    assert_eq!(claim_count, 3);

    // A second tick must be idempotent: no new claim tasks, no duplicate
    // volume.
    sub.tick().await.unwrap();
    assert_eq!(store.list_volumes().unwrap().len(), 1);
    let tasks2 = store.list_tasks().unwrap();
    let claim_count2 = tasks2
        .iter()
        .filter(|t| matches!(t.payload, TaskPayload::ClaimDisk { .. }))
        .count();
    assert_eq!(claim_count2, 3);
}

#[tokio::test]
async fn subscriber_skips_volume_when_pool_unknown() {
    let state = Arc::new(Mutex::new(FakeApi::default()));
    let port = run_fake_api(state.clone()).await;
    {
        let mut s = state.lock().await;
        s.pools = "[]".into();
        s.disks = "[]".into();
        s.volumes = r#"[{"uuid":"v","poolUuid":"missing","sizeBytes":4194304}]"#.into();
    }
    let store = temp_store();
    let sub = ApiSubscriber::new(
        ApiSubscriberConfig {
            base_url: format!("http://127.0.0.1:{port}"),
            poll_interval: Duration::from_secs(60),
            node_name: "test".into(),
        },
        store.clone(),
    )
    .unwrap();
    sub.tick().await.unwrap();
    assert_eq!(store.list_volumes().unwrap().len(), 0);
}
