//! redb-backed persistent store.
//!
//! Tables:
//!
//! * `pools`        — `pool_uuid` -> postcard(`Pool`)
//! * `disks`        — `disk_uuid` -> postcard(`Disk`)
//! * `volumes`      — `volume_uuid` -> postcard(`Volume`)
//! * `chunk_maps`   — `volume_uuid|chunk_index` (le bytes) -> postcard(`ChunkPlacement`)
//! * `tasks`        — `task_id` -> postcard(`Task`)
//!
//! Multi-table updates run inside a single redb write transaction, so the
//! store remains consistent across restarts even on torn writes.

use std::path::Path;
use std::sync::Arc;

use anyhow::{Context, Result};
use redb::{Database, ReadableDatabase, ReadableTable, TableDefinition};

use crate::types::{ChunkMap, ChunkPlacement, Disk, Pool, Task, Volume};

const POOLS: TableDefinition<&str, Vec<u8>> = TableDefinition::new("pools");
const DISKS: TableDefinition<&str, Vec<u8>> = TableDefinition::new("disks");
const VOLUMES: TableDefinition<&str, Vec<u8>> = TableDefinition::new("volumes");
const CHUNK_MAPS: TableDefinition<&[u8], Vec<u8>> = TableDefinition::new("chunk_maps");
const TASKS: TableDefinition<&str, Vec<u8>> = TableDefinition::new("tasks");

/// Persistent metadata store backed by redb.
#[derive(Clone)]
pub struct Store {
    db: Arc<Database>,
}

fn chunk_key(volume_uuid: &str, chunk_index: u32) -> Vec<u8> {
    let mut k = Vec::with_capacity(volume_uuid.len() + 1 + 4);
    k.extend_from_slice(volume_uuid.as_bytes());
    k.push(b'|');
    k.extend_from_slice(&chunk_index.to_be_bytes());
    k
}

impl Store {
    /// Open (and if needed, create) a store at `path`.
    pub fn open(path: impl AsRef<Path>) -> Result<Self> {
        let db = Database::create(path.as_ref()).context("open redb database")?;
        // Ensure tables exist.
        let wtx = db.begin_write()?;
        {
            let _ = wtx.open_table(POOLS)?;
            let _ = wtx.open_table(DISKS)?;
            let _ = wtx.open_table(VOLUMES)?;
            let _ = wtx.open_table(CHUNK_MAPS)?;
            let _ = wtx.open_table(TASKS)?;
        }
        wtx.commit()?;
        Ok(Self { db: Arc::new(db) })
    }

    // --- Pools -----------------------------------------------------------

    pub fn put_pool(&self, pool: &Pool) -> Result<()> {
        let bytes = postcard::to_allocvec(pool)?;
        let wtx = self.db.begin_write()?;
        {
            let mut t = wtx.open_table(POOLS)?;
            t.insert(pool.uuid.as_str(), bytes)?;
        }
        wtx.commit()?;
        Ok(())
    }

    pub fn get_pool(&self, uuid: &str) -> Result<Option<Pool>> {
        let rtx = self.db.begin_read()?;
        let t = rtx.open_table(POOLS)?;
        match t.get(uuid)? {
            Some(v) => Ok(Some(postcard::from_bytes(&v.value())?)),
            None => Ok(None),
        }
    }

    pub fn list_pools(&self) -> Result<Vec<Pool>> {
        let rtx = self.db.begin_read()?;
        let t = rtx.open_table(POOLS)?;
        let mut out = Vec::new();
        for r in t.iter()? {
            let (_, v) = r?;
            out.push(postcard::from_bytes(&v.value())?);
        }
        Ok(out)
    }

    pub fn delete_pool(&self, uuid: &str) -> Result<()> {
        let wtx = self.db.begin_write()?;
        {
            let mut t = wtx.open_table(POOLS)?;
            t.remove(uuid)?;
        }
        wtx.commit()?;
        Ok(())
    }

    // --- Disks -----------------------------------------------------------

    pub fn put_disk(&self, disk: &Disk) -> Result<()> {
        let bytes = postcard::to_allocvec(disk)?;
        let wtx = self.db.begin_write()?;
        {
            let mut t = wtx.open_table(DISKS)?;
            t.insert(disk.uuid.as_str(), bytes)?;
        }
        wtx.commit()?;
        Ok(())
    }

    pub fn get_disk(&self, uuid: &str) -> Result<Option<Disk>> {
        let rtx = self.db.begin_read()?;
        let t = rtx.open_table(DISKS)?;
        match t.get(uuid)? {
            Some(v) => Ok(Some(postcard::from_bytes(&v.value())?)),
            None => Ok(None),
        }
    }

    pub fn list_disks(&self) -> Result<Vec<Disk>> {
        let rtx = self.db.begin_read()?;
        let t = rtx.open_table(DISKS)?;
        let mut out = Vec::new();
        for r in t.iter()? {
            let (_, v) = r?;
            out.push(postcard::from_bytes(&v.value())?);
        }
        Ok(out)
    }

    pub fn list_disks_for_pool(&self, pool_uuid: &str) -> Result<Vec<Disk>> {
        Ok(self
            .list_disks()?
            .into_iter()
            .filter(|d| d.pool_uuid == pool_uuid)
            .collect())
    }

    pub fn delete_disk(&self, uuid: &str) -> Result<()> {
        let wtx = self.db.begin_write()?;
        {
            let mut t = wtx.open_table(DISKS)?;
            t.remove(uuid)?;
        }
        wtx.commit()?;
        Ok(())
    }

    // --- Volumes ---------------------------------------------------------

    pub fn put_volume(&self, vol: &Volume) -> Result<()> {
        let bytes = postcard::to_allocvec(vol)?;
        let wtx = self.db.begin_write()?;
        {
            let mut t = wtx.open_table(VOLUMES)?;
            t.insert(vol.uuid.as_str(), bytes)?;
        }
        wtx.commit()?;
        Ok(())
    }

    pub fn get_volume(&self, uuid: &str) -> Result<Option<Volume>> {
        let rtx = self.db.begin_read()?;
        let t = rtx.open_table(VOLUMES)?;
        match t.get(uuid)? {
            Some(v) => Ok(Some(postcard::from_bytes(&v.value())?)),
            None => Ok(None),
        }
    }

    pub fn list_volumes(&self) -> Result<Vec<Volume>> {
        let rtx = self.db.begin_read()?;
        let t = rtx.open_table(VOLUMES)?;
        let mut out = Vec::new();
        for r in t.iter()? {
            let (_, v) = r?;
            out.push(postcard::from_bytes(&v.value())?);
        }
        Ok(out)
    }

    pub fn delete_volume(&self, uuid: &str) -> Result<()> {
        let wtx = self.db.begin_write()?;
        {
            {
                let mut t = wtx.open_table(VOLUMES)?;
                t.remove(uuid)?;
            }
            // Remove all chunk map entries for this volume.
            let mut chunks = wtx.open_table(CHUNK_MAPS)?;
            let prefix = uuid.as_bytes().to_vec();
            let mut keys_to_remove = Vec::new();
            for r in chunks.iter()? {
                let (k, _) = r?;
                let kbytes = k.value().to_vec();
                if kbytes.len() > prefix.len()
                    && kbytes[..prefix.len()] == prefix[..]
                    && kbytes[prefix.len()] == b'|'
                {
                    keys_to_remove.push(kbytes);
                }
            }
            for k in keys_to_remove {
                chunks.remove(k.as_slice())?;
            }
        }
        wtx.commit()?;
        Ok(())
    }

    // --- Chunk maps ------------------------------------------------------

    /// Replace the chunk map for `volume_uuid` with `placements`. Atomic: any
    /// previous chunk map for that volume is wiped first.
    pub fn put_chunk_map(&self, volume_uuid: &str, placements: &[ChunkPlacement]) -> Result<()> {
        let wtx = self.db.begin_write()?;
        {
            let mut t = wtx.open_table(CHUNK_MAPS)?;
            // Wipe existing entries for volume.
            let prefix = volume_uuid.as_bytes().to_vec();
            let mut to_remove = Vec::new();
            for r in t.iter()? {
                let (k, _) = r?;
                let kb = k.value().to_vec();
                if kb.len() > prefix.len()
                    && kb[..prefix.len()] == prefix[..]
                    && kb[prefix.len()] == b'|'
                {
                    to_remove.push(kb);
                }
            }
            for k in to_remove {
                t.remove(k.as_slice())?;
            }
            for p in placements {
                let key = chunk_key(volume_uuid, p.index);
                let bytes = postcard::to_allocvec(p)?;
                t.insert(key.as_slice(), bytes)?;
            }
        }
        wtx.commit()?;
        Ok(())
    }

    pub fn get_chunk_map(&self, volume_uuid: &str) -> Result<ChunkMap> {
        let rtx = self.db.begin_read()?;
        let t = rtx.open_table(CHUNK_MAPS)?;
        let mut chunks = Vec::new();
        let prefix = volume_uuid.as_bytes().to_vec();
        for r in t.iter()? {
            let (k, v) = r?;
            let kb = k.value().to_vec();
            if kb.len() > prefix.len()
                && kb[..prefix.len()] == prefix[..]
                && kb[prefix.len()] == b'|'
            {
                let p: ChunkPlacement = postcard::from_bytes(&v.value())?;
                chunks.push(p);
            }
        }
        chunks.sort_by_key(|c| c.index);
        Ok(ChunkMap {
            volume_uuid: volume_uuid.to_string(),
            chunks,
        })
    }

    /// Update a single chunk placement (e.g. after a replication task acks).
    pub fn put_chunk_placement(&self, volume_uuid: &str, placement: &ChunkPlacement) -> Result<()> {
        let wtx = self.db.begin_write()?;
        {
            let mut t = wtx.open_table(CHUNK_MAPS)?;
            let key = chunk_key(volume_uuid, placement.index);
            let bytes = postcard::to_allocvec(placement)?;
            t.insert(key.as_slice(), bytes)?;
        }
        wtx.commit()?;
        Ok(())
    }

    // --- Tasks -----------------------------------------------------------

    pub fn put_task(&self, task: &Task) -> Result<()> {
        let bytes = postcard::to_allocvec(task)?;
        let wtx = self.db.begin_write()?;
        {
            let mut t = wtx.open_table(TASKS)?;
            t.insert(task.id.as_str(), bytes)?;
        }
        wtx.commit()?;
        Ok(())
    }

    pub fn list_tasks(&self) -> Result<Vec<Task>> {
        let rtx = self.db.begin_read()?;
        let t = rtx.open_table(TASKS)?;
        let mut out: Vec<Task> = Vec::new();
        for r in t.iter()? {
            let (_, v) = r?;
            out.push(postcard::from_bytes(&v.value())?);
        }
        // Stable order by created_unix_secs then id.
        out.sort_by(|a, b| {
            a.created_unix_secs
                .cmp(&b.created_unix_secs)
                .then_with(|| a.id.cmp(&b.id))
        });
        Ok(out)
    }

    pub fn delete_task(&self, id: &str) -> Result<()> {
        let wtx = self.db.begin_write()?;
        {
            let mut t = wtx.open_table(TASKS)?;
            t.remove(id)?;
        }
        wtx.commit()?;
        Ok(())
    }

    pub fn task_exists_for(&self, predicate: impl Fn(&Task) -> bool) -> Result<bool> {
        Ok(self.list_tasks()?.iter().any(predicate))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::types::{ProtectionSpec, TaskPayload};

    fn tempstore() -> Store {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("store.redb");
        // tempdir is dropped after function but Database keeps file open;
        // leak the dir for the test's lifetime.
        let leaked: &'static tempfile::TempDir = Box::leak(Box::new(dir));
        let _ = leaked.path();
        Store::open(path).unwrap()
    }

    #[test]
    fn pools_roundtrip() {
        let s = tempstore();
        let p = Pool {
            uuid: "p1".into(),
            name: "pool1".into(),
            replication_factor: 2,
            tier: "fast".into(),
            generation: 1,
        };
        s.put_pool(&p).unwrap();
        assert_eq!(s.get_pool("p1").unwrap().unwrap(), p);
        assert_eq!(s.list_pools().unwrap().len(), 1);
        s.delete_pool("p1").unwrap();
        assert!(s.get_pool("p1").unwrap().is_none());
    }

    #[test]
    fn disks_roundtrip() {
        let s = tempstore();
        let d = Disk {
            uuid: "d1".into(),
            node: "node1".into(),
            device_path: "/dev/nvme0n1".into(),
            size_bytes: 1024,
            pool_uuid: "p1".into(),
            state: "ready".into(),
            tier: "fast".into(),
            present: true,
            generation: 1,
        };
        s.put_disk(&d).unwrap();
        assert_eq!(s.get_disk("d1").unwrap().unwrap(), d);
        assert_eq!(s.list_disks_for_pool("p1").unwrap().len(), 1);
    }

    #[test]
    fn volumes_and_chunk_maps_atomic_delete() {
        let s = tempstore();
        let v = Volume {
            uuid: "v1".into(),
            name: "vol1".into(),
            pool_uuid: "p1".into(),
            size_bytes: 16 * 1024 * 1024,
            protection: ProtectionSpec {
                replication_factor: 2,
            },
            chunk_size_bytes: 4 * 1024 * 1024,
            chunk_count: 4,
            generation: 1,
        };
        s.put_volume(&v).unwrap();
        let placements = (0u32..4)
            .map(|i| ChunkPlacement {
                index: i,
                chunk_id: String::new(),
                disk_uuids: vec![format!("d{i}-a"), format!("d{i}-b")],
            })
            .collect::<Vec<_>>();
        s.put_chunk_map("v1", &placements).unwrap();
        let cm = s.get_chunk_map("v1").unwrap();
        assert_eq!(cm.chunks.len(), 4);
        s.delete_volume("v1").unwrap();
        assert_eq!(s.get_chunk_map("v1").unwrap().chunks.len(), 0);
    }

    #[test]
    fn tasks_roundtrip_and_order() {
        let s = tempstore();
        let t1 = Task {
            id: "a".into(),
            created_unix_secs: 100,
            payload: TaskPayload::ReleaseDisk {
                disk_uuid: "d1".into(),
            },
        };
        let t2 = Task {
            id: "b".into(),
            created_unix_secs: 50,
            payload: TaskPayload::ReleaseDisk {
                disk_uuid: "d2".into(),
            },
        };
        s.put_task(&t1).unwrap();
        s.put_task(&t2).unwrap();
        let tasks = s.list_tasks().unwrap();
        assert_eq!(tasks[0].id, "b");
        assert_eq!(tasks[1].id, "a");
        s.delete_task("a").unwrap();
        assert_eq!(s.list_tasks().unwrap().len(), 1);
    }

    #[test]
    fn put_chunk_map_replaces_previous_entries() {
        let s = tempstore();
        let p1 = vec![
            ChunkPlacement {
                index: 0,
                chunk_id: String::new(),
                disk_uuids: vec!["a".into()],
            },
            ChunkPlacement {
                index: 1,
                chunk_id: String::new(),
                disk_uuids: vec!["b".into()],
            },
        ];
        s.put_chunk_map("v", &p1).unwrap();
        let p2 = vec![ChunkPlacement {
            index: 0,
            chunk_id: String::new(),
            disk_uuids: vec!["x".into()],
        }];
        s.put_chunk_map("v", &p2).unwrap();
        let cm = s.get_chunk_map("v").unwrap();
        assert_eq!(cm.chunks.len(), 1);
        assert_eq!(cm.chunks[0].disk_uuids, vec!["x".to_string()]);
    }
}
