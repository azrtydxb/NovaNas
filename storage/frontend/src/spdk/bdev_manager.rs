//! SPDK bdev manager (placeholder port).
//!
//! Production lookups against the SPDK bdev table go through
//! `dataplane/src/spdk/bdev_manager.rs`. This placeholder keeps the
//! frontend compilable with `--features spdk-sys` and tracks created
//! bdev names in-process so the lifecycle code can be exercised end-
//! to-end with the placeholder reactor.

use std::collections::HashMap;
use std::sync::Mutex;

use crate::error::{FrontendError, Result};

#[derive(Debug, Clone)]
pub struct BdevInfo {
    pub name: String,
    pub block_size: u32,
    pub num_blocks: u64,
    pub bdev_type: String,
}

pub struct BdevManager {
    bdevs: Mutex<HashMap<String, BdevInfo>>,
}

impl Default for BdevManager {
    fn default() -> Self {
        Self::new()
    }
}

impl BdevManager {
    pub fn new() -> Self {
        Self {
            bdevs: Mutex::new(HashMap::new()),
        }
    }

    pub fn register(&self, info: BdevInfo) -> Result<()> {
        let mut g = self.bdevs.lock().unwrap();
        if g.contains_key(&info.name) {
            return Err(FrontendError::Bdev(format!(
                "bdev {} already exists",
                info.name
            )));
        }
        g.insert(info.name.clone(), info);
        Ok(())
    }

    pub fn unregister(&self, name: &str) -> Result<()> {
        self.bdevs
            .lock()
            .unwrap()
            .remove(name)
            .ok_or_else(|| FrontendError::Bdev(format!("bdev {} not found", name)))?;
        Ok(())
    }

    pub fn list(&self) -> Vec<BdevInfo> {
        self.bdevs.lock().unwrap().values().cloned().collect()
    }
}
