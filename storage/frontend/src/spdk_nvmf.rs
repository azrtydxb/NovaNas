//! SPDK NVMe-oF target wrapper (placeholder port).
//!
//! Production version sits at `dataplane/src/spdk/nvmf_manager.rs`. The
//! frontend will own its own NVMe-oF target once Agent C completes the
//! split. Today this module exposes the same `NvmfTarget` surface as
//! the no-op double, so the rest of the daemon links and runs.

use std::collections::HashMap;

use async_trait::async_trait;
use tokio::sync::Mutex;

use crate::error::Result;
use crate::nvmf::{NvmfTarget, SubsystemSpec};

pub struct SpdkNvmfTarget {
    listen_address: String,
    listen_port: u16,
    state: Mutex<HashMap<String, SubsystemSpec>>,
}

impl SpdkNvmfTarget {
    pub fn new(listen_address: impl Into<String>, listen_port: u16) -> Self {
        Self {
            listen_address: listen_address.into(),
            listen_port,
            state: Mutex::new(Default::default()),
        }
    }

    pub fn listen_address(&self) -> &str {
        &self.listen_address
    }

    pub fn listen_port(&self) -> u16 {
        self.listen_port
    }
}

#[async_trait]
impl NvmfTarget for SpdkNvmfTarget {
    async fn add_subsystem(&self, spec: &SubsystemSpec) -> Result<()> {
        log::warn!(
            "SpdkNvmfTarget::add_subsystem(volume={}, bdev={}): SPDK port pending",
            spec.volume_name,
            spec.bdev_name
        );
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
