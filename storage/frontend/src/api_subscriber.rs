//! HTTP subscriber for `BlockVolume` lifecycle events from `novanas-api`.
//!
//! The API server (TypeScript Fastify, Postgres-backed — out of scope for
//! this crate) is the user-facing CRUD surface. It does not push events;
//! the frontend polls `/api/v1/block-volumes` on a small interval and
//! reconciles the local set of NVMe-oF subsystems against the returned
//! list. New volumes get a bdev + subsystem; deleted volumes get torn
//! down.
//!
//! This module is intentionally framework-light: a `BlockVolumeReconciler`
//! trait represents the side-effects, and tests inject a fake reconciler
//! and a `serde_json::Value` API response.

use std::collections::HashSet;
use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use serde::{Deserialize, Serialize};

use crate::error::{FrontendError, Result};

/// A BlockVolume as exposed by the API. The schema is intentionally
/// permissive — only the fields the frontend actually consumes are
/// captured. Extras are tolerated and ignored.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ApiBlockVolume {
    pub name: String,
    #[serde(default)]
    pub pool: String,
    #[serde(default)]
    pub size_bytes: u64,
    #[serde(default)]
    pub phase: String,
}

/// API response wrapper. The API server returns either a JSON array or
/// a `{ "items": [...] }` envelope; we accept both.
#[derive(Debug, Deserialize)]
#[serde(untagged)]
enum BlockVolumeListEnvelope {
    Array(Vec<ApiBlockVolume>),
    Items { items: Vec<ApiBlockVolume> },
}

/// Reconciler called by the subscriber when volumes appear / disappear.
#[async_trait]
pub trait BlockVolumeReconciler: Send + Sync {
    async fn on_volume_added(&self, vol: &ApiBlockVolume) -> Result<()>;
    async fn on_volume_removed(&self, name: &str) -> Result<()>;
}

/// HTTP-based source of BlockVolume snapshots.
#[async_trait]
pub trait BlockVolumeSource: Send + Sync {
    async fn list(&self) -> Result<Vec<ApiBlockVolume>>;
}

/// Production HTTP source backed by `reqwest`.
pub struct HttpBlockVolumeSource {
    base_url: String,
    client: reqwest::Client,
}

impl HttpBlockVolumeSource {
    pub fn new(base_url: impl Into<String>) -> Self {
        Self {
            base_url: base_url.into(),
            client: reqwest::Client::builder()
                .timeout(Duration::from_secs(10))
                .build()
                .expect("reqwest client build"),
        }
    }
}

#[async_trait]
impl BlockVolumeSource for HttpBlockVolumeSource {
    async fn list(&self) -> Result<Vec<ApiBlockVolume>> {
        let url = format!(
            "{}/api/v1/block-volumes",
            self.base_url.trim_end_matches('/')
        );
        let resp = self.client.get(&url).send().await?;
        if !resp.status().is_success() {
            return Err(FrontendError::Api(format!(
                "GET {} -> {}",
                url,
                resp.status()
            )));
        }
        let env: BlockVolumeListEnvelope = resp.json().await?;
        match env {
            BlockVolumeListEnvelope::Array(v) => Ok(v),
            BlockVolumeListEnvelope::Items { items } => Ok(items),
        }
    }
}

/// The subscriber loop: poll `source` every `interval`, diff against the
/// last snapshot, and call into `reconciler`.
pub struct ApiSubscriber {
    source: Arc<dyn BlockVolumeSource>,
    reconciler: Arc<dyn BlockVolumeReconciler>,
    interval: Duration,
}

impl ApiSubscriber {
    pub fn new(
        source: Arc<dyn BlockVolumeSource>,
        reconciler: Arc<dyn BlockVolumeReconciler>,
        interval: Duration,
    ) -> Self {
        Self {
            source,
            reconciler,
            interval,
        }
    }

    /// Run a single reconcile pass against the given previous-name set.
    /// Returns the new set of names so the caller can continue ticking.
    pub async fn tick(&self, known: &mut HashSet<String>) -> Result<()> {
        let current = self.source.list().await?;
        let current_names: HashSet<String> = current.iter().map(|v| v.name.clone()).collect();

        // Additions.
        for vol in &current {
            if !known.contains(&vol.name) {
                if let Err(e) = self.reconciler.on_volume_added(vol).await {
                    log::warn!("reconciler add {} failed: {}", vol.name, e);
                    continue;
                }
                known.insert(vol.name.clone());
            }
        }
        // Removals.
        let removed: Vec<String> = known
            .iter()
            .filter(|n| !current_names.contains(*n))
            .cloned()
            .collect();
        for name in removed {
            if let Err(e) = self.reconciler.on_volume_removed(&name).await {
                log::warn!("reconciler remove {} failed: {}", name, e);
                continue;
            }
            known.remove(&name);
        }
        Ok(())
    }

    /// Long-running poll loop. Errors are logged and retried on the next
    /// tick — this loop only returns on cancellation.
    pub async fn run(self: Arc<Self>) {
        let mut known: HashSet<String> = HashSet::new();
        let mut ticker = tokio::time::interval(self.interval);
        loop {
            ticker.tick().await;
            if let Err(e) = self.tick(&mut known).await {
                log::warn!("api_subscriber: tick failed: {}", e);
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct FakeSource(std::sync::Mutex<Vec<Vec<ApiBlockVolume>>>);
    #[async_trait]
    impl BlockVolumeSource for FakeSource {
        async fn list(&self) -> Result<Vec<ApiBlockVolume>> {
            let mut g = self.0.lock().unwrap();
            if g.is_empty() {
                return Ok(Vec::new());
            }
            Ok(g.remove(0))
        }
    }

    struct CountingReconciler {
        added: std::sync::Mutex<Vec<String>>,
        removed: std::sync::Mutex<Vec<String>>,
    }
    #[async_trait]
    impl BlockVolumeReconciler for CountingReconciler {
        async fn on_volume_added(&self, vol: &ApiBlockVolume) -> Result<()> {
            self.added.lock().unwrap().push(vol.name.clone());
            Ok(())
        }
        async fn on_volume_removed(&self, name: &str) -> Result<()> {
            self.removed.lock().unwrap().push(name.to_string());
            Ok(())
        }
    }

    #[tokio::test]
    async fn tick_diffs_additions_and_removals() {
        let source = Arc::new(FakeSource(std::sync::Mutex::new(vec![
            vec![
                ApiBlockVolume {
                    name: "a".into(),
                    pool: "p".into(),
                    size_bytes: 10,
                    phase: "Ready".into(),
                },
                ApiBlockVolume {
                    name: "b".into(),
                    pool: "p".into(),
                    size_bytes: 20,
                    phase: "Ready".into(),
                },
            ],
            vec![ApiBlockVolume {
                name: "b".into(),
                pool: "p".into(),
                size_bytes: 20,
                phase: "Ready".into(),
            }],
            vec![],
        ])));
        let rec = Arc::new(CountingReconciler {
            added: Default::default(),
            removed: Default::default(),
        });
        let sub = ApiSubscriber::new(
            source as Arc<dyn BlockVolumeSource>,
            rec.clone() as Arc<dyn BlockVolumeReconciler>,
            Duration::from_millis(1),
        );
        let mut known = HashSet::new();

        sub.tick(&mut known).await.unwrap();
        assert_eq!(known.len(), 2);

        sub.tick(&mut known).await.unwrap();
        assert_eq!(known.len(), 1);
        assert!(known.contains("b"));

        sub.tick(&mut known).await.unwrap();
        assert!(known.is_empty());

        let added = rec.added.lock().unwrap().clone();
        let removed = rec.removed.lock().unwrap().clone();
        assert_eq!(added, vec!["a".to_string(), "b".to_string()]);
        assert_eq!(removed, vec!["a".to_string(), "b".to_string()]);
    }

    #[test]
    fn envelope_accepts_array_or_items() {
        let arr: BlockVolumeListEnvelope = serde_json::from_str(r#"[{"name":"x"}]"#).unwrap();
        let items: BlockVolumeListEnvelope =
            serde_json::from_str(r#"{"items":[{"name":"y"}]}"#).unwrap();
        match arr {
            BlockVolumeListEnvelope::Array(v) => assert_eq!(v[0].name, "x"),
            _ => panic!("array form should match"),
        }
        match items {
            BlockVolumeListEnvelope::Items { items } => assert_eq!(items[0].name, "y"),
            _ => panic!("items form should match"),
        }
    }
}
