//! End-to-end integration test for the data daemon's meta-side pipeline.
//!
//! Spins up an in-process `MetaService` over a Unix-domain socket, hands
//! the daemon's [`MetaClient`] to a [`TaskRunner`], feeds a sequence of
//! tasks (a CLAIM_DISK, a CHUNK_OP, then an unsupported one), and
//! asserts:
//!   - the right handler fires for each task,
//!   - acks propagate back to the fake meta with the expected
//!     success / failure status,
//!   - poll_tasks transient errors don't kill the runner.
//!
//! No SPDK headers are required; everything stays in tokio.

use std::path::PathBuf;
use std::sync::Arc;
use std::sync::Mutex;
use std::time::Duration;

use async_trait::async_trait;
use tokio::net::UnixListener;
use tonic::{Request, Response, Status};

use novanas_dataplane::backend::chunk_store::{ChunkHeader, ChunkStore, ChunkStoreStats};
use novanas_dataplane::error::{DataPlaneError, Result as DpResult};
use novanas_dataplane::meta_client::{MetaClient, MetaClientConfig};
use novanas_dataplane::policy::operations::ChunkOperations;
use novanas_dataplane::task_handlers::HandlerContext;
use novanas_dataplane::task_runner::{ShutdownToken, TaskRunner, TaskRunnerConfig};
use novanas_dataplane::transport::meta_proto::{
    meta_service_server::{MetaService, MetaServiceServer},
    AckTaskRequest, ChunkMapSlice, ChunkOpTask, ClaimDiskRequest, ClaimDiskTask,
    CreateVolumeRequest, Disk, DiskList, DiskRef, GetChunkMapRequest, HeartbeatRequest,
    HeartbeatResponse, ListDisksRequest, PollTasksRequest, Pool, PoolList, PoolRef, Task,
    TaskBatch, TaskKind, Volume, VolumeList, VolumeRef,
};

/// In-memory chunk store the test handler is wired against.
struct MemStore {
    inner: Mutex<std::collections::HashMap<String, Vec<u8>>>,
}

impl MemStore {
    fn new() -> Self {
        Self {
            inner: Mutex::new(Default::default()),
        }
    }
}

#[async_trait]
impl ChunkStore for MemStore {
    async fn put(&self, chunk_id: &str, data: &[u8]) -> DpResult<()> {
        self.inner
            .lock()
            .unwrap()
            .insert(chunk_id.to_string(), data.to_vec());
        Ok(())
    }
    async fn get(&self, chunk_id: &str) -> DpResult<Vec<u8>> {
        self.inner
            .lock()
            .unwrap()
            .get(chunk_id)
            .cloned()
            .ok_or_else(|| DataPlaneError::ChunkEngineError(format!("missing {chunk_id}")))
    }
    async fn delete(&self, chunk_id: &str) -> DpResult<()> {
        self.inner
            .lock()
            .unwrap()
            .remove(chunk_id)
            .ok_or_else(|| DataPlaneError::ChunkEngineError(format!("missing {chunk_id}")))?;
        Ok(())
    }
    async fn exists(&self, chunk_id: &str) -> DpResult<bool> {
        Ok(self.inner.lock().unwrap().contains_key(chunk_id))
    }
    async fn stats(&self) -> DpResult<ChunkStoreStats> {
        Ok(ChunkStoreStats {
            backend_name: "mem".into(),
            total_bytes: 0,
            used_bytes: 0,
            data_bytes: 0,
            chunk_count: self.inner.lock().unwrap().len() as u64,
        })
    }
}

/// Recorded task ack.
#[derive(Debug, Clone)]
struct RecordedAck {
    task_id: String,
    success: bool,
    error_message: String,
}

/// Fake `MetaService` whose poll queue and ack log are inspectable.
struct FakeMeta {
    /// Pre-loaded queue of `TaskBatch`es to hand to PollTasks. The first
    /// entry is consumed on each call. Once empty, we return an empty
    /// batch (long-poll deadline simulation kept short).
    queue: Mutex<Vec<TaskBatch>>,
    /// Acks recorded in the order received.
    acks: Mutex<Vec<RecordedAck>>,
    /// Counter — first N PollTasks calls fail with `Unavailable`.
    transient_failures: Mutex<u32>,
}

#[async_trait]
impl MetaService for FakeMeta {
    async fn put_pool(&self, _r: Request<Pool>) -> std::result::Result<Response<()>, Status> {
        Ok(Response::new(()))
    }
    async fn get_pool(&self, _r: Request<PoolRef>) -> std::result::Result<Response<Pool>, Status> {
        Err(Status::unimplemented(""))
    }
    async fn list_pools(&self, _r: Request<()>) -> std::result::Result<Response<PoolList>, Status> {
        Ok(Response::new(PoolList { items: vec![] }))
    }
    async fn delete_pool(&self, _r: Request<PoolRef>) -> std::result::Result<Response<()>, Status> {
        Ok(Response::new(()))
    }
    async fn put_disk(&self, _r: Request<Disk>) -> std::result::Result<Response<()>, Status> {
        Ok(Response::new(()))
    }
    async fn get_disk(&self, _r: Request<DiskRef>) -> std::result::Result<Response<Disk>, Status> {
        Err(Status::unimplemented(""))
    }
    async fn list_disks(
        &self,
        _r: Request<ListDisksRequest>,
    ) -> std::result::Result<Response<DiskList>, Status> {
        Ok(Response::new(DiskList { items: vec![] }))
    }
    async fn delete_disk(&self, _r: Request<DiskRef>) -> std::result::Result<Response<()>, Status> {
        Ok(Response::new(()))
    }
    async fn create_volume(
        &self,
        _r: Request<CreateVolumeRequest>,
    ) -> std::result::Result<Response<Volume>, Status> {
        Err(Status::unimplemented(""))
    }
    async fn get_volume(
        &self,
        _r: Request<VolumeRef>,
    ) -> std::result::Result<Response<Volume>, Status> {
        Err(Status::unimplemented(""))
    }
    async fn list_volumes(
        &self,
        _r: Request<()>,
    ) -> std::result::Result<Response<VolumeList>, Status> {
        Ok(Response::new(VolumeList { items: vec![] }))
    }
    async fn delete_volume(
        &self,
        _r: Request<VolumeRef>,
    ) -> std::result::Result<Response<()>, Status> {
        Ok(Response::new(()))
    }
    async fn get_chunk_map(
        &self,
        _r: Request<GetChunkMapRequest>,
    ) -> std::result::Result<Response<ChunkMapSlice>, Status> {
        Ok(Response::new(ChunkMapSlice { entries: vec![] }))
    }
    async fn claim_disk(
        &self,
        _r: Request<ClaimDiskRequest>,
    ) -> std::result::Result<Response<Disk>, Status> {
        Ok(Response::new(Disk::default()))
    }
    async fn release_disk(
        &self,
        _r: Request<DiskRef>,
    ) -> std::result::Result<Response<()>, Status> {
        Ok(Response::new(()))
    }
    async fn poll_tasks(
        &self,
        _r: Request<PollTasksRequest>,
    ) -> std::result::Result<Response<TaskBatch>, Status> {
        // Maybe inject transient failure first.
        {
            let mut tf = self.transient_failures.lock().unwrap();
            if *tf > 0 {
                *tf -= 1;
                return Err(Status::unavailable("temporary"));
            }
        }
        let next_batch = {
            let mut q = self.queue.lock().unwrap();
            if q.is_empty() {
                None
            } else {
                Some(q.remove(0))
            }
        };
        match next_batch {
            Some(batch) => Ok(Response::new(batch)),
            None => {
                tokio::time::sleep(Duration::from_millis(50)).await;
                Ok(Response::new(TaskBatch { items: vec![] }))
            }
        }
    }
    async fn ack_task(
        &self,
        r: Request<AckTaskRequest>,
    ) -> std::result::Result<Response<()>, Status> {
        let req = r.into_inner();
        self.acks.lock().unwrap().push(RecordedAck {
            task_id: req.task_id,
            success: req.success,
            error_message: req.error_message,
        });
        Ok(Response::new(()))
    }
    async fn heartbeat(
        &self,
        _r: Request<HeartbeatRequest>,
    ) -> std::result::Result<Response<HeartbeatResponse>, Status> {
        Ok(Response::new(HeartbeatResponse {
            desired_crush_digest: vec![],
            pending_task_count: 0,
        }))
    }
}

fn fake_meta() -> Arc<FakeMeta> {
    Arc::new(FakeMeta {
        queue: Mutex::new(Vec::new()),
        acks: Mutex::new(Vec::new()),
        transient_failures: Mutex::new(0),
    })
}

async fn spawn_meta(socket: PathBuf, meta: Arc<FakeMeta>) -> tokio::task::JoinHandle<()> {
    let _ = std::fs::remove_file(&socket);
    if let Some(parent) = socket.parent() {
        std::fs::create_dir_all(parent).unwrap();
    }
    let listener = UnixListener::bind(&socket).expect("bind fake meta socket");
    let incoming = tokio_stream::wrappers::UnixListenerStream::new(listener);
    let svc = MetaServiceServer::new(FakeMetaWrap(meta));
    tokio::spawn(async move {
        let _ = tonic::transport::Server::builder()
            .add_service(svc)
            .serve_with_incoming(incoming)
            .await;
    })
}

/// Newtype wrapper because `MetaService` is implemented on FakeMeta and
/// tonic wants ownership of the service value.
struct FakeMetaWrap(Arc<FakeMeta>);

#[async_trait]
impl MetaService for FakeMetaWrap {
    async fn put_pool(&self, r: Request<Pool>) -> std::result::Result<Response<()>, Status> {
        self.0.put_pool(r).await
    }
    async fn get_pool(&self, r: Request<PoolRef>) -> std::result::Result<Response<Pool>, Status> {
        self.0.get_pool(r).await
    }
    async fn list_pools(&self, r: Request<()>) -> std::result::Result<Response<PoolList>, Status> {
        self.0.list_pools(r).await
    }
    async fn delete_pool(&self, r: Request<PoolRef>) -> std::result::Result<Response<()>, Status> {
        self.0.delete_pool(r).await
    }
    async fn put_disk(&self, r: Request<Disk>) -> std::result::Result<Response<()>, Status> {
        self.0.put_disk(r).await
    }
    async fn get_disk(&self, r: Request<DiskRef>) -> std::result::Result<Response<Disk>, Status> {
        self.0.get_disk(r).await
    }
    async fn list_disks(
        &self,
        r: Request<ListDisksRequest>,
    ) -> std::result::Result<Response<DiskList>, Status> {
        self.0.list_disks(r).await
    }
    async fn delete_disk(&self, r: Request<DiskRef>) -> std::result::Result<Response<()>, Status> {
        self.0.delete_disk(r).await
    }
    async fn create_volume(
        &self,
        r: Request<CreateVolumeRequest>,
    ) -> std::result::Result<Response<Volume>, Status> {
        self.0.create_volume(r).await
    }
    async fn get_volume(
        &self,
        r: Request<VolumeRef>,
    ) -> std::result::Result<Response<Volume>, Status> {
        self.0.get_volume(r).await
    }
    async fn list_volumes(
        &self,
        r: Request<()>,
    ) -> std::result::Result<Response<VolumeList>, Status> {
        self.0.list_volumes(r).await
    }
    async fn delete_volume(
        &self,
        r: Request<VolumeRef>,
    ) -> std::result::Result<Response<()>, Status> {
        self.0.delete_volume(r).await
    }
    async fn get_chunk_map(
        &self,
        r: Request<GetChunkMapRequest>,
    ) -> std::result::Result<Response<ChunkMapSlice>, Status> {
        self.0.get_chunk_map(r).await
    }
    async fn claim_disk(
        &self,
        r: Request<ClaimDiskRequest>,
    ) -> std::result::Result<Response<Disk>, Status> {
        self.0.claim_disk(r).await
    }
    async fn release_disk(&self, r: Request<DiskRef>) -> std::result::Result<Response<()>, Status> {
        self.0.release_disk(r).await
    }
    async fn poll_tasks(
        &self,
        r: Request<PollTasksRequest>,
    ) -> std::result::Result<Response<TaskBatch>, Status> {
        self.0.poll_tasks(r).await
    }
    async fn ack_task(
        &self,
        r: Request<AckTaskRequest>,
    ) -> std::result::Result<Response<()>, Status> {
        self.0.ack_task(r).await
    }
    async fn heartbeat(
        &self,
        r: Request<HeartbeatRequest>,
    ) -> std::result::Result<Response<HeartbeatResponse>, Status> {
        self.0.heartbeat(r).await
    }
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
async fn runner_dispatches_tasks_and_acks() {
    let dir = tempfile::tempdir().unwrap();
    let socket = dir.path().join("meta.sock");
    let meta = fake_meta();

    // Put a chunk into the in-memory store the chunk_op handler will use.
    let store = Arc::new(MemStore::new());
    let chunk_id = "abc".repeat(22) + "ab";
    store.put(&chunk_id, &chunk_payload(b"hi")).await.unwrap();

    // Pre-load the queue: a CHUNK_OP that should succeed and an
    // unspecified task that must be acked as failure.
    let success_task = Task {
        id: "task-1".into(),
        kind: TaskKind::ReplicateChunk as i32,
        node_id: "node-a".into(),
        created_at: None,
        attempt: 0,
        claim_disk: None,
        release_disk: None,
        chunk_op: Some(ChunkOpTask {
            chunk_id: chunk_id.clone(),
            volume_uuid: "v".into(),
            chunk_index: 0,
            src_disk_uuid: "node-a".into(),
            dst_disk_uuids: vec!["node-a".into()],
        }),
    };
    let bad_task = Task {
        id: "task-2".into(),
        kind: TaskKind::Unspecified as i32,
        node_id: "node-a".into(),
        created_at: None,
        attempt: 0,
        claim_disk: None,
        release_disk: None,
        chunk_op: None,
    };
    meta.queue.lock().unwrap().push(TaskBatch {
        items: vec![success_task, bad_task],
    });
    // Two transient PollTasks failures before the queue is consumed.
    *meta.transient_failures.lock().unwrap() = 2;

    let _meta_handle = spawn_meta(socket.clone(), meta.clone()).await;
    // Give the listener a moment.
    tokio::time::sleep(Duration::from_millis(50)).await;

    let client = MetaClient::connect(MetaClientConfig {
        socket_path: socket.clone(),
        connect_timeout: Duration::from_secs(2),
        rpc_timeout: Duration::from_secs(2),
    })
    .await
    .expect("meta client connects");

    let ops = Arc::new(ChunkOperations::new("node-a".into(), store.clone()));
    let ctx = Arc::new(HandlerContext::new("node-a").with_chunk_ops(ops));
    let runner_cfg = TaskRunnerConfig {
        node_id: "node-a".into(),
        max_tasks: 8,
        deadline_ms: 250,
        concurrency: 4,
        error_backoff: Duration::from_millis(20),
    };
    let runner = TaskRunner::new(runner_cfg, ctx);
    let shutdown = ShutdownToken::new();
    let runner_handle = {
        let s = shutdown.clone();
        tokio::spawn(async move { runner.run(client, s).await })
    };

    // Wait until both acks land or timeout.
    let deadline = tokio::time::Instant::now() + Duration::from_secs(5);
    loop {
        let n = meta.acks.lock().unwrap().len();
        if n >= 2 {
            break;
        }
        if tokio::time::Instant::now() > deadline {
            panic!("acks did not arrive in time, got {n}");
        }
        tokio::time::sleep(Duration::from_millis(50)).await;
    }

    shutdown.cancel();
    let _ = tokio::time::timeout(Duration::from_secs(3), runner_handle).await;

    let acks = meta.acks.lock().unwrap().clone();
    let task1 = acks.iter().find(|a| a.task_id == "task-1").unwrap();
    assert!(task1.success, "REPLICATE_CHUNK should ack success");
    let task2 = acks.iter().find(|a| a.task_id == "task-2").unwrap();
    assert!(!task2.success, "Unspecified task should ack failure");
    assert!(task2.error_message.contains("unspecified"));
}

#[tokio::test]
async fn runner_handles_claim_disk_task() {
    let dir = tempfile::tempdir().unwrap();
    let socket = dir.path().join("meta.sock");
    let meta = fake_meta();

    // Build a fake disk file that the claim-handler can write to. We
    // arrange a sysfs root with one block device whose dev path resolves
    // to a local file we control.
    let sysfs_root = dir.path().join("sys");
    let block = sysfs_root.join("block").join("sda");
    std::fs::create_dir_all(block.join("queue")).unwrap();
    std::fs::create_dir_all(block.join("device")).unwrap();
    std::fs::write(block.join("size"), "128").unwrap(); // 128 * 512 = 64 KiB
    std::fs::write(block.join("queue/rotational"), "1").unwrap();
    std::fs::write(block.join("device/model"), "FakeDisk").unwrap();
    std::fs::write(block.join("device/serial"), "FAKE-001").unwrap();
    // The handler resolves dev path at /dev/<slot> when it builds its
    // SysfsClaimBackend; we can't redirect /dev in a unit test, so we
    // run the claim-disk path via the unit-test backend in the
    // task_handlers module directly. Here we only verify that runner
    // routes the task to the handler — the failure case (no /dev/sda)
    // produces a non-success ack we observe at meta.

    let task = Task {
        id: "claim-1".into(),
        kind: TaskKind::ClaimDisk as i32,
        node_id: "node-a".into(),
        created_at: None,
        attempt: 0,
        claim_disk: Some(ClaimDiskTask {
            pool_uuid: "pool-1".into(),
            pool_name: "hdd".into(),
            disk_uuid: "sda:FAKE-001".into(),
            role: 1,
            crush_digest: vec![],
            force: false,
        }),
        release_disk: None,
        chunk_op: None,
    };
    meta.queue
        .lock()
        .unwrap()
        .push(TaskBatch { items: vec![task] });

    let _meta_handle = spawn_meta(socket.clone(), meta.clone()).await;
    tokio::time::sleep(Duration::from_millis(50)).await;

    let client = MetaClient::connect(MetaClientConfig {
        socket_path: socket.clone(),
        connect_timeout: Duration::from_secs(2),
        rpc_timeout: Duration::from_secs(2),
    })
    .await
    .unwrap();

    let ctx = Arc::new(HandlerContext::new("node-a").with_sysfs_root(sysfs_root.clone()));
    let runner_cfg = TaskRunnerConfig {
        node_id: "node-a".into(),
        max_tasks: 4,
        deadline_ms: 250,
        concurrency: 2,
        error_backoff: Duration::from_millis(20),
    };
    let runner = TaskRunner::new(runner_cfg, ctx);
    let shutdown = ShutdownToken::new();
    let runner_handle = {
        let s = shutdown.clone();
        tokio::spawn(async move { runner.run(client, s).await })
    };

    let deadline = tokio::time::Instant::now() + Duration::from_secs(5);
    loop {
        let n = meta.acks.lock().unwrap().len();
        if n >= 1 {
            break;
        }
        if tokio::time::Instant::now() > deadline {
            panic!("claim ack did not arrive");
        }
        tokio::time::sleep(Duration::from_millis(50)).await;
    }
    shutdown.cancel();
    let _ = tokio::time::timeout(Duration::from_secs(3), runner_handle).await;

    let acks = meta.acks.lock().unwrap().clone();
    let claim = acks.iter().find(|a| a.task_id == "claim-1").unwrap();
    // The dev path /dev/sda does not exist in CI sandboxes, so we expect
    // a failure ack carrying a meaningful error message.
    assert!(!claim.success);
    assert!(
        !claim.error_message.is_empty(),
        "expected error_message, got empty"
    );
}
