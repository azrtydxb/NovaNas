//! HTTP poller against `novanas-api`.
//!
//! On each tick:
//! 1. Fetch `/api/v1/pools`, `/api/v1/disks`, `/api/v1/block-volumes`.
//! 2. Reconcile into the local redb store: PutPool / PutDisk / CreateVolume.
//! 3. When `Disk.spec.pool` changes (or a previously-unclaimed disk moves into
//!    a pool), enqueue a `ClaimDiskTask`.
//! 4. When a new BlockVolume appears, run CRUSH and persist the chunk map.

use std::time::Duration;

use anyhow::{Context, Result};
use serde::Deserialize;
use tracing::{debug, info, warn};

use crate::crush;
use crate::server::pool_topology;
use crate::store::Store;
use crate::types::{ChunkPlacement, Disk, Pool, ProtectionSpec, Task, TaskPayload, Volume};
use crate::CHUNK_SIZE_BYTES;

#[derive(Debug, Clone)]
pub struct ApiSubscriberConfig {
    pub base_url: String,
    pub poll_interval: Duration,
    pub node_name: String,
}

impl Default for ApiSubscriberConfig {
    fn default() -> Self {
        Self {
            base_url: "http://localhost:3000".to_string(),
            poll_interval: Duration::from_secs(5),
            node_name: "local".to_string(),
        }
    }
}

// --- API DTOs (loose: only fields we care about) -------------------------

#[derive(Debug, Deserialize, Default)]
struct ApiPool {
    #[serde(default)]
    uuid: String,
    #[serde(default)]
    name: String,
    #[serde(default, alias = "replicationFactor", alias = "replication_factor")]
    replication_factor: Option<u32>,
    #[serde(default)]
    tier: Option<String>,
}

#[derive(Debug, Deserialize, Default)]
struct ApiDisk {
    #[serde(default)]
    uuid: String,
    #[serde(default)]
    #[allow(dead_code)]
    name: String,
    #[serde(default)]
    node: Option<String>,
    #[serde(default, alias = "devicePath", alias = "device_path")]
    device_path: Option<String>,
    #[serde(default, alias = "sizeBytes", alias = "size_bytes")]
    size_bytes: Option<u64>,
    #[serde(default, alias = "poolUuid", alias = "pool", alias = "pool_uuid")]
    pool_uuid: Option<String>,
    #[serde(default)]
    state: Option<String>,
    #[serde(default)]
    tier: Option<String>,
    #[serde(default)]
    present: Option<bool>,
}

#[derive(Debug, Deserialize, Default)]
struct ApiProtection {
    #[serde(default, alias = "replicationFactor", alias = "replication_factor")]
    replication_factor: Option<u32>,
}

#[derive(Debug, Deserialize, Default)]
struct ApiBlockVolume {
    #[serde(default)]
    uuid: String,
    #[serde(default)]
    name: String,
    #[serde(default, alias = "poolUuid", alias = "pool", alias = "pool_uuid")]
    pool_uuid: Option<String>,
    #[serde(default, alias = "sizeBytes", alias = "size_bytes")]
    size_bytes: Option<u64>,
    #[serde(default)]
    protection: Option<ApiProtection>,
}

/// API subscriber. Owns an HTTP client + the local store.
pub struct ApiSubscriber {
    cfg: ApiSubscriberConfig,
    http: reqwest::Client,
    store: Store,
}

impl ApiSubscriber {
    pub fn new(cfg: ApiSubscriberConfig, store: Store) -> Result<Self> {
        let http = reqwest::Client::builder()
            .timeout(Duration::from_secs(10))
            .build()?;
        Ok(Self { cfg, http, store })
    }

    /// Run the loop forever. Cancellable via the parent task being dropped.
    pub async fn run(self) -> Result<()> {
        let mut iv = tokio::time::interval(self.cfg.poll_interval);
        iv.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
        loop {
            iv.tick().await;
            if let Err(e) = self.tick().await {
                warn!(error = %e, "api subscriber tick failed");
            }
        }
    }

    /// Single reconciliation pass. Public for tests.
    pub async fn tick(&self) -> Result<()> {
        self.reconcile_pools().await?;
        self.reconcile_disks().await?;
        self.reconcile_volumes().await?;
        Ok(())
    }

    async fn fetch_json<T: for<'de> Deserialize<'de>>(&self, path: &str) -> Result<Vec<T>> {
        let url = format!("{}{}", self.cfg.base_url.trim_end_matches('/'), path);
        let resp = self
            .http
            .get(&url)
            .send()
            .await
            .with_context(|| format!("GET {url}"))?;
        if !resp.status().is_success() {
            anyhow::bail!("GET {url} returned {}", resp.status());
        }
        let bytes = resp.bytes().await?;
        // Accept both `{"items": [...]}` and a raw array.
        if let Ok(items) = serde_json::from_slice::<Vec<T>>(&bytes) {
            return Ok(items);
        }
        #[derive(Deserialize)]
        struct Wrap<T> {
            #[serde(default = "Vec::new")]
            items: Vec<T>,
        }
        let w: Wrap<T> = serde_json::from_slice(&bytes)
            .with_context(|| format!("decode response from {url}"))?;
        Ok(w.items)
    }

    async fn reconcile_pools(&self) -> Result<()> {
        let api_pools: Vec<ApiPool> = self.fetch_json("/api/v1/pools").await?;
        for ap in api_pools {
            if ap.uuid.is_empty() {
                continue;
            }
            let new_p = Pool {
                uuid: ap.uuid.clone(),
                name: if ap.name.is_empty() {
                    ap.uuid.clone()
                } else {
                    ap.name
                },
                replication_factor: ap.replication_factor.unwrap_or(2).max(1),
                tier: ap.tier.unwrap_or_default(),
                generation: 1,
            };
            let cur = self.store.get_pool(&new_p.uuid)?;
            match cur {
                Some(c) if pools_equivalent(&c, &new_p) => {}
                Some(c) => {
                    let mut updated = new_p.clone();
                    updated.generation = c.generation + 1;
                    self.store.put_pool(&updated)?;
                    debug!(uuid = %updated.uuid, "pool updated");
                }
                None => {
                    self.store.put_pool(&new_p)?;
                    info!(uuid = %new_p.uuid, "pool added");
                }
            }
        }
        Ok(())
    }

    async fn reconcile_disks(&self) -> Result<()> {
        let api_disks: Vec<ApiDisk> = self.fetch_json("/api/v1/disks").await?;
        for ad in api_disks {
            if ad.uuid.is_empty() {
                continue;
            }
            let new_pool = ad.pool_uuid.clone().unwrap_or_default();
            let new_state = ad.state.clone().unwrap_or_else(|| {
                if new_pool.is_empty() {
                    "unclaimed".into()
                } else {
                    "claiming".into()
                }
            });
            let new_d = Disk {
                uuid: ad.uuid.clone(),
                node: ad
                    .node
                    .clone()
                    .unwrap_or_else(|| self.cfg.node_name.clone()),
                device_path: ad.device_path.clone().unwrap_or_default(),
                size_bytes: ad.size_bytes.unwrap_or(0),
                pool_uuid: new_pool.clone(),
                state: new_state,
                tier: ad.tier.clone().unwrap_or_default(),
                present: ad.present.unwrap_or(true),
                generation: 1,
            };
            let cur = self.store.get_disk(&new_d.uuid)?;
            match cur {
                Some(c) if disks_equivalent(&c, &new_d) => {}
                Some(c) => {
                    let mut updated = new_d.clone();
                    updated.generation = c.generation + 1;
                    // Preserve state if newer assignment matches existing.
                    self.store.put_disk(&updated)?;
                    if c.pool_uuid != new_d.pool_uuid && !new_d.pool_uuid.is_empty() {
                        self.enqueue_claim(&updated)?;
                    }
                }
                None => {
                    self.store.put_disk(&new_d)?;
                    if !new_d.pool_uuid.is_empty() {
                        self.enqueue_claim(&new_d)?;
                    }
                    info!(uuid = %new_d.uuid, "disk added");
                }
            }
        }
        Ok(())
    }

    fn enqueue_claim(&self, d: &Disk) -> Result<()> {
        // Suppress duplicate ClaimDisk tasks for the same disk.
        let already = self.store.task_exists_for(|t| {
            matches!(&t.payload, TaskPayload::ClaimDisk { disk_uuid, .. } if disk_uuid == &d.uuid)
        })?;
        if already {
            return Ok(());
        }
        let task = Task {
            id: uuid::Uuid::new_v4().to_string(),
            created_unix_secs: now_secs(),
            payload: TaskPayload::ClaimDisk {
                disk_uuid: d.uuid.clone(),
                pool_uuid: d.pool_uuid.clone(),
            },
        };
        self.store.put_task(&task)?;
        info!(disk = %d.uuid, pool = %d.pool_uuid, "claim task enqueued");
        Ok(())
    }

    async fn reconcile_volumes(&self) -> Result<()> {
        let api_vols: Vec<ApiBlockVolume> = self.fetch_json("/api/v1/block-volumes").await?;
        for av in api_vols {
            if av.uuid.is_empty() {
                continue;
            }
            let pool_uuid = av.pool_uuid.clone().unwrap_or_default();
            if pool_uuid.is_empty() {
                continue;
            }
            if self.store.get_volume(&av.uuid)?.is_some() {
                continue; // volumes are immutable from the meta side
            }
            let pool = match self.store.get_pool(&pool_uuid)? {
                Some(p) => p,
                None => {
                    warn!(volume = %av.uuid, pool = %pool_uuid, "volume references unknown pool, skipping");
                    continue;
                }
            };
            let size = av.size_bytes.unwrap_or(0);
            if size == 0 {
                warn!(volume = %av.uuid, "volume size_bytes=0, skipping");
                continue;
            }
            let rf = av
                .protection
                .as_ref()
                .and_then(|p| p.replication_factor)
                .unwrap_or(pool.replication_factor)
                .max(1);
            let topo = pool_topology(&self.store, &pool_uuid)?;
            if (topo.len() as u32) < rf {
                warn!(
                    volume = %av.uuid,
                    pool = %pool_uuid,
                    have = topo.len(),
                    need = rf,
                    "not enough disks for volume yet"
                );
                continue;
            }
            let chunk_count = crate::chunk_count_for(size);
            let mut placements = Vec::with_capacity(chunk_count as usize);
            for i in 0..chunk_count {
                let key = crush::chunk_key(&av.uuid, i as u32);
                let disks = match crush::select(&key, rf as usize, &topo) {
                    Ok(v) => v,
                    Err(e) => {
                        warn!(volume = %av.uuid, error = %e, "CRUSH placement failed");
                        return Ok(());
                    }
                };
                placements.push(ChunkPlacement {
                    index: i as u32,
                    chunk_id: String::new(),
                    disk_uuids: disks,
                });
            }
            let v = Volume {
                uuid: av.uuid.clone(),
                name: if av.name.is_empty() {
                    av.uuid.clone()
                } else {
                    av.name
                },
                pool_uuid,
                size_bytes: size,
                protection: ProtectionSpec {
                    replication_factor: rf,
                },
                chunk_size_bytes: CHUNK_SIZE_BYTES,
                chunk_count,
                generation: 1,
            };
            self.store.put_volume(&v)?;
            self.store.put_chunk_map(&v.uuid, &placements)?;
            info!(uuid = %v.uuid, chunks = chunk_count, "volume created via api subscriber");
        }
        Ok(())
    }
}

fn pools_equivalent(a: &Pool, b: &Pool) -> bool {
    a.uuid == b.uuid
        && a.name == b.name
        && a.replication_factor == b.replication_factor
        && a.tier == b.tier
}

fn disks_equivalent(a: &Disk, b: &Disk) -> bool {
    a.uuid == b.uuid
        && a.node == b.node
        && a.device_path == b.device_path
        && a.size_bytes == b.size_bytes
        && a.pool_uuid == b.pool_uuid
        && a.state == b.state
        && a.tier == b.tier
        && a.present == b.present
}

fn now_secs() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or_default()
}
