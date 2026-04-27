//! End-to-end store invariants: drive the redb store through a realistic
//! pool-create → claim-disks → create-volume → policy-tick → ack-tasks
//! sequence, asserting state transitions at each step.

use novanas_meta::policy::{PolicyChecker, PolicyConfig};
use novanas_meta::store::Store;
use novanas_meta::types::{ChunkPlacement, Disk, Pool, ProtectionSpec, Task, TaskPayload, Volume};

fn temp_store() -> Store {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("s.redb");
    let leaked: &'static tempfile::TempDir = Box::leak(Box::new(dir));
    let _ = leaked.path();
    Store::open(path).unwrap()
}

#[test]
fn full_lifecycle() {
    let s = temp_store();

    // 1. Pool created.
    let pool = Pool {
        uuid: "p".into(),
        name: "p".into(),
        replication_factor: 2,
        tier: "fast".into(),
        generation: 1,
    };
    s.put_pool(&pool).unwrap();
    assert_eq!(s.list_pools().unwrap().len(), 1);

    // 2. Three disks claimed.
    for i in 0..3 {
        s.put_disk(&Disk {
            uuid: format!("d{i}"),
            node: "local".into(),
            device_path: format!("/dev/x{i}"),
            size_bytes: 10 * 1024 * 1024 * 1024,
            pool_uuid: "p".into(),
            state: "ready".into(),
            tier: "fast".into(),
            present: true,
            generation: 1,
        })
        .unwrap();
    }
    assert_eq!(s.list_disks_for_pool("p").unwrap().len(), 3);

    // 3. Volume + chunk map populated (4 chunks, replication 2).
    let v = Volume {
        uuid: "v".into(),
        name: "v".into(),
        pool_uuid: "p".into(),
        size_bytes: 16 * 1024 * 1024,
        protection: ProtectionSpec {
            replication_factor: 2,
        },
        chunk_size_bytes: 4 * 1024 * 1024,
        chunk_count: 4,
        generation: 1,
    };
    s.put_volume(&v).unwrap();
    let placements: Vec<_> = (0u32..4)
        .map(|i| ChunkPlacement {
            index: i,
            chunk_id: String::new(),
            // Place each chunk on d0 + d1 (compliant).
            disk_uuids: vec!["d0".into(), "d1".into()],
        })
        .collect();
    s.put_chunk_map("v", &placements).unwrap();
    let cm = s.get_chunk_map("v").unwrap();
    assert_eq!(cm.chunks.len(), 4);

    // 4. Policy tick — fully compliant — emits nothing.
    let p = PolicyChecker::new(PolicyConfig::default(), s.clone());
    p.tick().unwrap();
    assert_eq!(s.list_tasks().unwrap().len(), 0);

    // 5. Mark d1 absent → under-replicated.
    let mut d1 = s.get_disk("d1").unwrap().unwrap();
    d1.present = false;
    s.put_disk(&d1).unwrap();
    p.tick().unwrap();
    let tasks = s.list_tasks().unwrap();
    assert_eq!(tasks.len(), 4, "expected one ReplicateChunk task per chunk");
    for t in &tasks {
        match &t.payload {
            TaskPayload::ReplicateChunk {
                volume_uuid,
                target_disk_uuids,
                ..
            } => {
                assert_eq!(volume_uuid, "v");
                assert_eq!(target_disk_uuids.len(), 2);
            }
            _ => panic!("unexpected task payload"),
        }
    }

    // 6. Idempotent: a second tick does NOT double-emit.
    p.tick().unwrap();
    assert_eq!(s.list_tasks().unwrap().len(), 4);

    // 7. Ack all tasks → tasks gone.
    let ids: Vec<String> = tasks.iter().map(|t| t.id.clone()).collect();
    for id in ids {
        s.delete_task(&id).unwrap();
    }
    assert_eq!(s.list_tasks().unwrap().len(), 0);

    // 8. Volume deletion clears chunk map atomically.
    s.delete_volume("v").unwrap();
    assert_eq!(s.get_chunk_map("v").unwrap().chunks.len(), 0);
    assert!(s.get_volume("v").unwrap().is_none());
}

#[test]
fn task_ordering_is_stable() {
    let s = temp_store();
    for (i, ts) in [(0u64, 100), (1, 50), (2, 200)].iter().enumerate() {
        s.put_task(&Task {
            id: format!("t{i}"),
            created_unix_secs: ts.1,
            payload: TaskPayload::ReleaseDisk {
                disk_uuid: format!("d{i}"),
            },
        })
        .unwrap();
        let _ = ts.0;
    }
    let tasks = s.list_tasks().unwrap();
    let times: Vec<u64> = tasks.iter().map(|t| t.created_unix_secs).collect();
    assert_eq!(times, vec![50, 100, 200]);
}
