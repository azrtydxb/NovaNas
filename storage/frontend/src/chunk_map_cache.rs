//! Local cache of (volume_name, chunk_index) → chunk_id.
//!
//! The chunk-map is the authoritative output of meta's CRUSH+policy. The
//! frontend caches it locally so that hot reads don't pay a meta
//! round-trip per chunk. Two stores are supported:
//!
//!  * In-memory only (`ChunkMapCache::in_memory`): lost on restart, used
//!    by tests and useful in CI.
//!  * redb-backed (`ChunkMapCache::open`): a single file under
//!    `/var/lib/novanas/frontend/chunk_map.redb`. Persists across
//!    restarts; reseeded from meta as misses happen.
//!
//! Invalidation is range-based: `invalidate_volume(name)` drops every
//! cached entry for a volume. Use this on a volume-config change pushed
//! via the API subscriber, or when meta's heartbeat hints at staleness.

use std::path::Path;
use std::sync::Arc;

use dashmap::DashMap;
use redb::{Database, ReadableDatabase, ReadableTable, TableDefinition};

use crate::error::{FrontendError, Result};
use crate::meta_client::MetaClient;

const CHUNK_MAP_TABLE: TableDefinition<&str, &str> = TableDefinition::new("chunk_map_v1");

/// Cache key: composite of volume_name + chunk_index.
fn key(volume: &str, chunk_index: u64) -> String {
    format!("{}#{}", volume, chunk_index)
}

/// Local chunk-map cache with optional persistence.
pub struct ChunkMapCache {
    mem: DashMap<String, String>,
    db: Option<Arc<Database>>,
    meta: tokio::sync::RwLock<Option<Arc<dyn MetaClient>>>,
}

impl ChunkMapCache {
    /// Build a process-local in-memory cache. Used for tests + dev.
    pub fn in_memory() -> Self {
        Self {
            mem: DashMap::new(),
            db: None,
            meta: tokio::sync::RwLock::new(None),
        }
    }

    /// Open or create a redb-backed cache file.
    pub fn open<P: AsRef<Path>>(path: P) -> Result<Self> {
        let db = Database::create(path.as_ref())
            .map_err(|e| FrontendError::ChunkMapCache(format!("open redb: {}", e)))?;
        // Make sure the table exists.
        let txn = db
            .begin_write()
            .map_err(|e| FrontendError::ChunkMapCache(format!("begin_write: {}", e)))?;
        {
            let _ = txn
                .open_table(CHUNK_MAP_TABLE)
                .map_err(|e| FrontendError::ChunkMapCache(format!("open_table: {}", e)))?;
        }
        txn.commit()
            .map_err(|e| FrontendError::ChunkMapCache(format!("commit: {}", e)))?;
        Ok(Self {
            mem: DashMap::new(),
            db: Some(Arc::new(db)),
            meta: tokio::sync::RwLock::new(None),
        })
    }

    /// Wire a meta client used for cache misses. May be set after creation
    /// because the meta client itself can be expensive to build at boot
    /// (e.g. waits for the UDS to appear).
    pub async fn set_meta(&self, meta: Arc<dyn MetaClient>) {
        *self.meta.write().await = Some(meta);
    }

    /// Persist the entry both in-memory and (if present) on disk.
    pub fn record_chunk_id(&self, volume: &str, chunk_index: u64, chunk_id: &str) {
        let k = key(volume, chunk_index);
        self.mem.insert(k.clone(), chunk_id.to_string());
        if let Some(db) = &self.db {
            // Persist failures are logged but never failed the write — the
            // in-memory state is authoritative for the current process.
            if let Err(e) = self.persist(db, &k, chunk_id) {
                log::warn!("chunk_map_cache: persist {} failed: {}", k, e);
            }
        }
    }

    fn persist(&self, db: &Database, k: &str, v: &str) -> Result<()> {
        let txn = db
            .begin_write()
            .map_err(|e| FrontendError::ChunkMapCache(format!("begin_write: {}", e)))?;
        {
            let mut t = txn
                .open_table(CHUNK_MAP_TABLE)
                .map_err(|e| FrontendError::ChunkMapCache(format!("open_table: {}", e)))?;
            t.insert(k, v)
                .map_err(|e| FrontendError::ChunkMapCache(format!("insert: {}", e)))?;
        }
        txn.commit()
            .map_err(|e| FrontendError::ChunkMapCache(format!("commit: {}", e)))?;
        Ok(())
    }

    fn read_persistent(&self, k: &str) -> Result<Option<String>> {
        let Some(db) = &self.db else { return Ok(None) };
        let txn = db
            .begin_read()
            .map_err(|e| FrontendError::ChunkMapCache(format!("begin_read: {}", e)))?;
        let t = txn
            .open_table(CHUNK_MAP_TABLE)
            .map_err(|e| FrontendError::ChunkMapCache(format!("open_table: {}", e)))?;
        let got = t
            .get(k)
            .map_err(|e| FrontendError::ChunkMapCache(format!("get: {}", e)))?;
        Ok(got.map(|v| v.value().to_string()))
    }

    /// Look up purely from the local cache — never calls meta.
    pub fn lookup(&self, volume: &str, chunk_index: u64) -> Option<String> {
        let k = key(volume, chunk_index);
        if let Some(v) = self.mem.get(&k) {
            return Some(v.clone());
        }
        // Try the persistent layer; promote into memory on hit.
        match self.read_persistent(&k) {
            Ok(Some(v)) => {
                self.mem.insert(k, v.clone());
                Some(v)
            }
            _ => None,
        }
    }

    /// Lookup with an automatic meta fall-back. Returns `Ok(None)` only
    /// when meta replies with no entry for this chunk index (i.e. the
    /// chunk has never been written).
    pub async fn lookup_or_fetch(&self, volume: &str, chunk_index: u64) -> Result<Option<String>> {
        if let Some(v) = self.lookup(volume, chunk_index) {
            return Ok(Some(v));
        }
        // Miss — pull a small range covering this index from meta.
        let meta_opt = self.meta.read().await.clone();
        let Some(meta) = meta_opt else {
            return Ok(None);
        };
        let slice = meta
            .get_chunk_map(volume, chunk_index, chunk_index + 1)
            .await?;
        let mut found = None;
        for entry in slice.entries {
            if !entry.chunk_id.is_empty() {
                self.record_chunk_id(volume, entry.chunk_index, &entry.chunk_id);
                if entry.chunk_index == chunk_index {
                    found = Some(entry.chunk_id);
                }
            }
        }
        Ok(found)
    }

    /// Drop every cached entry (memory + disk) for a volume.
    pub fn invalidate_volume(&self, volume: &str) -> Result<usize> {
        let prefix = format!("{}#", volume);
        // Memory.
        let mut removed = 0usize;
        let keys: Vec<String> = self
            .mem
            .iter()
            .filter(|kv| kv.key().starts_with(&prefix))
            .map(|kv| kv.key().clone())
            .collect();
        for k in &keys {
            if self.mem.remove(k).is_some() {
                removed += 1;
            }
        }

        // Disk.
        if let Some(db) = &self.db {
            let txn = db
                .begin_write()
                .map_err(|e| FrontendError::ChunkMapCache(format!("begin_write: {}", e)))?;
            {
                let mut t = txn
                    .open_table(CHUNK_MAP_TABLE)
                    .map_err(|e| FrontendError::ChunkMapCache(format!("open_table: {}", e)))?;
                let to_drop: Vec<String> = {
                    let r = t
                        .iter()
                        .map_err(|e| FrontendError::ChunkMapCache(format!("iter: {}", e)))?;
                    r.flat_map(|res| res.ok())
                        .map(|(k, _)| k.value().to_string())
                        .filter(|k| k.starts_with(&prefix))
                        .collect()
                };
                for k in to_drop {
                    t.remove(k.as_str())
                        .map_err(|e| FrontendError::ChunkMapCache(format!("remove: {}", e)))?;
                }
            }
            txn.commit()
                .map_err(|e| FrontendError::ChunkMapCache(format!("commit: {}", e)))?;
        }
        Ok(removed)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::meta::{
        ChunkMapEntry, ChunkMapSlice, HeartbeatRequest, HeartbeatResponse, Volume, VolumeList,
    };
    use async_trait::async_trait;

    struct StubMeta {
        responses: std::sync::Mutex<Vec<ChunkMapSlice>>,
    }
    impl StubMeta {
        fn with(slice: ChunkMapSlice) -> Self {
            Self {
                responses: std::sync::Mutex::new(vec![slice]),
            }
        }
    }
    #[async_trait]
    impl MetaClient for StubMeta {
        async fn get_volume(&self, _: &str) -> Result<Volume> {
            Err(FrontendError::Meta("stub: no GetVolume".into()))
        }
        async fn list_volumes(&self) -> Result<VolumeList> {
            Ok(VolumeList { items: vec![] })
        }
        async fn get_chunk_map(&self, _: &str, _: u64, _: u64) -> Result<ChunkMapSlice> {
            let mut g = self.responses.lock().unwrap();
            Ok(g.pop().unwrap_or(ChunkMapSlice { entries: vec![] }))
        }
        async fn heartbeat(&self, _: HeartbeatRequest) -> Result<HeartbeatResponse> {
            Ok(HeartbeatResponse {
                desired_crush_digest: vec![],
                pending_task_count: 0,
            })
        }
    }

    #[test]
    fn in_memory_record_and_lookup() {
        let c = ChunkMapCache::in_memory();
        c.record_chunk_id("vol", 7, "cid-7");
        assert_eq!(c.lookup("vol", 7).unwrap(), "cid-7");
        assert!(c.lookup("vol", 8).is_none());
    }

    #[test]
    fn persistent_record_survives_reopen() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("cache.redb");
        {
            let c = ChunkMapCache::open(&path).unwrap();
            c.record_chunk_id("vol", 1, "cid-1");
            c.record_chunk_id("vol", 2, "cid-2");
        }
        let c2 = ChunkMapCache::open(&path).unwrap();
        // Memory is cold; lookup should still hit the redb file.
        assert_eq!(c2.lookup("vol", 1).unwrap(), "cid-1");
        assert_eq!(c2.lookup("vol", 2).unwrap(), "cid-2");
    }

    #[tokio::test]
    async fn miss_consults_meta_and_records() {
        let c = ChunkMapCache::in_memory();
        c.set_meta(Arc::new(StubMeta::with(ChunkMapSlice {
            entries: vec![ChunkMapEntry {
                chunk_index: 9,
                chunk_id: "deadbeef".into(),
                disk_wwns: vec!["w".into()],
            }],
        })))
        .await;
        let got = c.lookup_or_fetch("vol", 9).await.unwrap();
        assert_eq!(got.unwrap(), "deadbeef");
        // Subsequent lookup should hit the in-memory cache.
        assert_eq!(c.lookup("vol", 9).unwrap(), "deadbeef");
    }

    #[test]
    fn invalidate_drops_volume_entries() {
        let c = ChunkMapCache::in_memory();
        c.record_chunk_id("v1", 0, "a");
        c.record_chunk_id("v1", 1, "b");
        c.record_chunk_id("v2", 0, "z");
        let removed = c.invalidate_volume("v1").unwrap();
        assert_eq!(removed, 2);
        assert!(c.lookup("v1", 0).is_none());
        assert_eq!(c.lookup("v2", 0).unwrap(), "z");
    }
}
