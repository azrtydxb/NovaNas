//! NVMe-oF target management.
//!
//! Real SPDK-backed creation/teardown of NVMe-oF subsystems lives behind
//! the `spdk-sys` feature in `spdk_target.rs`. This module always
//! defines the `NvmfTarget` trait so callers (api_subscriber's
//! reconciler, tests) link without needing SPDK.
//!
//! Each `BlockVolume` owns one subsystem with NQN
//! `nqn.2024-01.io.novanas:volume-<name>`. The single TCP listener is
//! shared.

use async_trait::async_trait;

use crate::error::Result;

/// NQN convention used for volume subsystems. Must stay stable —
/// initiators rely on it for discovery.
pub fn volume_nqn(volume_name: &str) -> String {
    format!("nqn.2024-01.io.novanas:volume-{}", volume_name)
}

/// Configuration for a freshly-created subsystem.
#[derive(Debug, Clone)]
pub struct SubsystemSpec {
    pub volume_name: String,
    pub size_bytes: u64,
    pub bdev_name: String,
    pub listen_address: String,
    pub listen_port: u16,
}

impl SubsystemSpec {
    pub fn nqn(&self) -> String {
        volume_nqn(&self.volume_name)
    }
}

/// Trait for creating + tearing down NVMe-oF subsystems.
///
/// Production impl (`spdk_target::SpdkNvmfTarget`) sits behind the
/// `spdk-sys` feature. A `NoopNvmfTarget` is always available for tests
/// and for builds without SPDK linkage.
#[async_trait]
pub trait NvmfTarget: Send + Sync {
    async fn add_subsystem(&self, spec: &SubsystemSpec) -> Result<()>;
    async fn remove_subsystem(&self, volume_name: &str) -> Result<()>;
    async fn list_subsystems(&self) -> Result<Vec<String>>;
}

/// In-process target double — records spec changes for tests.
pub struct NoopNvmfTarget {
    state: tokio::sync::Mutex<std::collections::HashMap<String, SubsystemSpec>>,
}

impl NoopNvmfTarget {
    pub fn new() -> Self {
        Self {
            state: tokio::sync::Mutex::new(Default::default()),
        }
    }
}

impl Default for NoopNvmfTarget {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl NvmfTarget for NoopNvmfTarget {
    async fn add_subsystem(&self, spec: &SubsystemSpec) -> Result<()> {
        self.state
            .lock()
            .await
            .insert(spec.volume_name.clone(), spec.clone());
        Ok(())
    }
    async fn remove_subsystem(&self, volume_name: &str) -> Result<()> {
        self.state.lock().await.remove(volume_name);
        Ok(())
    }
    async fn list_subsystems(&self) -> Result<Vec<String>> {
        Ok(self.state.lock().await.keys().cloned().collect())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn nqn_format_is_stable() {
        assert_eq!(volume_nqn("alpha"), "nqn.2024-01.io.novanas:volume-alpha");
    }

    #[tokio::test]
    async fn noop_target_records_state() {
        let t = NoopNvmfTarget::new();
        t.add_subsystem(&SubsystemSpec {
            volume_name: "v1".into(),
            size_bytes: 1024,
            bdev_name: "novanas-v1".into(),
            listen_address: "0.0.0.0".into(),
            listen_port: 4420,
        })
        .await
        .unwrap();
        let l = t.list_subsystems().await.unwrap();
        assert_eq!(l, vec!["v1".to_string()]);
        t.remove_subsystem("v1").await.unwrap();
        assert!(t.list_subsystems().await.unwrap().is_empty());
    }
}
