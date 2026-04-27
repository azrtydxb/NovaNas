//! Integration test: HTTP-driven BlockVolume reconciliation.
//!
//! Stands up a tiny `hyper` server that returns a JSON body, points the
//! frontend's `HttpBlockVolumeSource` at it, runs one tick through the
//! `VolumeReconciler`, and asserts that bdev + nvmf state changed.

use std::collections::HashSet;
use std::convert::Infallible;
use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use http_body_util::Full;
use hyper::body::Bytes;
use hyper::service::service_fn;
use hyper::Response;
use hyper_util::rt::TokioIo;
use tokio::net::TcpListener;

use novanas_frontend::api_subscriber::{ApiSubscriber, HttpBlockVolumeSource};
use novanas_frontend::chunk_engine::ChunkEngine;
use novanas_frontend::chunk_map_cache::ChunkMapCache;
use novanas_frontend::error::Result;
use novanas_frontend::meta_client::MetaClient;
use novanas_frontend::ndp_client::NdpChunkClient;
use novanas_frontend::nvmf::{NoopNvmfTarget, NvmfTarget};
use novanas_frontend::proto::meta::{
    ChunkMapSlice, HeartbeatRequest, HeartbeatResponse, Volume, VolumeList,
};
use novanas_frontend::reconciler::VolumeReconciler;
use novanas_frontend::volume_bdev::{NoopVolumeBdevManager, VolumeBdevManager};

struct FixedSizeMeta;
#[async_trait]
impl MetaClient for FixedSizeMeta {
    async fn get_volume(&self, name: &str) -> Result<Volume> {
        Ok(Volume {
            name: name.to_string(),
            uuid: "u".into(),
            pool_name: "p".into(),
            size_bytes: 4 * 1024 * 1024 * 32,
            chunk_size_bytes: 4 * 1024 * 1024,
            chunk_count: 32,
            protection: None,
            phase: "Ready".into(),
            created_at: None,
        })
    }
    async fn list_volumes(&self) -> Result<VolumeList> {
        Ok(VolumeList { items: vec![] })
    }
    async fn get_chunk_map(&self, _: &str, _: u64, _: u64) -> Result<ChunkMapSlice> {
        Ok(ChunkMapSlice { entries: vec![] })
    }
    async fn heartbeat(&self, _: HeartbeatRequest) -> Result<HeartbeatResponse> {
        Ok(HeartbeatResponse {
            desired_crush_digest: vec![],
            pending_task_count: 0,
        })
    }
}

#[derive(Clone)]
struct FakeApi {
    body: &'static str,
}

async fn serve(api: FakeApi) -> std::net::SocketAddr {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    tokio::spawn(async move {
        loop {
            let (stream, _) = match listener.accept().await {
                Ok(p) => p,
                Err(_) => return,
            };
            let api = api.clone();
            tokio::spawn(async move {
                let svc = service_fn(move |_req| {
                    let body = api.body.to_string();
                    async move {
                        Ok::<_, Infallible>(
                            Response::builder()
                                .header("content-type", "application/json")
                                .body(Full::new(Bytes::from(body)))
                                .unwrap(),
                        )
                    }
                });
                let io = TokioIo::new(stream);
                let _ = hyper::server::conn::http1::Builder::new()
                    .serve_connection(io, svc)
                    .await;
            });
        }
    });
    addr
}

struct FakeNdp;
#[async_trait]
impl NdpChunkClient for FakeNdp {
    async fn put_chunk(&self, _: &str, _: &[u8]) -> Result<()> {
        Ok(())
    }
    async fn get_chunk(&self, _: &str) -> Result<Vec<u8>> {
        Ok(vec![])
    }
    async fn delete_chunk(&self, _: &str) -> Result<()> {
        Ok(())
    }
}

#[tokio::test]
async fn http_volume_appears_then_disappears() {
    let api = FakeApi {
        body: r#"{"items":[{"name":"alpha","pool":"hot","size_bytes":131072,"phase":"Ready"}]}"#,
    };
    let addr = serve(api).await;

    let cache = Arc::new(ChunkMapCache::in_memory());
    let ndp: Arc<dyn NdpChunkClient> = Arc::new(FakeNdp);
    let engine = Arc::new(ChunkEngine::new(cache, ndp));
    let bdev = Arc::new(NoopVolumeBdevManager::new(engine));
    let nvmf = Arc::new(NoopNvmfTarget::new());
    let meta: Arc<dyn MetaClient> = Arc::new(FixedSizeMeta);
    let reconciler = Arc::new(VolumeReconciler::new(
        meta,
        bdev.clone() as Arc<dyn VolumeBdevManager>,
        nvmf.clone() as Arc<dyn NvmfTarget>,
        "0.0.0.0",
        4420,
    ));
    let source = Arc::new(HttpBlockVolumeSource::new(format!("http://{}", addr)));
    let sub = ApiSubscriber::new(source, reconciler.clone(), Duration::from_millis(10));
    let mut known = HashSet::new();
    sub.tick(&mut known).await.unwrap();
    assert!(known.contains("alpha"));
    assert_eq!(bdev.list().await.unwrap().len(), 1);
    assert_eq!(
        nvmf.list_subsystems().await.unwrap(),
        vec!["alpha".to_string()]
    );

    // Now flip the API to return an empty list.
    let api2 = FakeApi { body: r#"[]"# };
    let addr2 = serve(api2).await;
    let source2 = Arc::new(HttpBlockVolumeSource::new(format!("http://{}", addr2)));
    let sub2 = ApiSubscriber::new(source2, reconciler, Duration::from_millis(10));
    sub2.tick(&mut known).await.unwrap();
    assert!(!known.contains("alpha"));
    assert!(bdev.list().await.unwrap().is_empty());
    assert!(nvmf.list_subsystems().await.unwrap().is_empty());
}
