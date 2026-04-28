//! SPDK bdev manager — frontend bookkeeping for registered bdevs.
//!
//! Ported from `storage/dataplane/src/spdk/bdev_manager.rs`. The frontend
//! does not own lvol stores or local block devices (those live in the
//! data daemon), so this manager is intentionally narrow: it tracks
//! the custom novanas volume bdevs that `spdk::volume_bdev` registers
//! and exposes a couple of malloc / query helpers for tests and bring-up.

use crate::error::{FrontendError, Result};
use log::info;
use std::collections::HashMap;
use std::os::raw::c_char;
use std::sync::Mutex;

use crate::spdk::reactor_dispatch;

#[allow(
    non_camel_case_types,
    non_snake_case,
    non_upper_case_globals,
    dead_code,
    improper_ctypes
)]
mod ffi {
    include!(concat!(env!("OUT_DIR"), "/spdk_bindings.rs"));
}

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

    pub fn get(&self, name: &str) -> Option<BdevInfo> {
        self.bdevs.lock().unwrap().get(name).cloned()
    }

    /// Create a malloc bdev (used during bring-up / tests). The bdev is
    /// registered in SPDK and tracked locally.
    pub fn create_malloc_bdev(
        &self,
        name: &str,
        size_mb: u64,
        block_size: u32,
    ) -> Result<BdevInfo> {
        info!("creating malloc bdev: name={}, size={}MB", name, size_mb);
        let total_bytes = size_mb * 1024 * 1024;
        let num_blocks = total_bytes / block_size as u64;

        reactor_dispatch::create_malloc_bdev(name, num_blocks, block_size)?;

        let (n_blocks, actual_bs) = reactor_dispatch::query_bdev(name)?;
        let info = BdevInfo {
            name: name.to_string(),
            block_size: actual_bs,
            num_blocks: n_blocks,
            bdev_type: "malloc".to_string(),
        };
        self.bdevs
            .lock()
            .unwrap()
            .insert(name.to_string(), info.clone());
        Ok(info)
    }

    /// Tear down an SPDK bdev by name.
    pub fn delete_bdev(&self, name: &str) -> Result<()> {
        info!("deleting bdev: {}", name);

        let name_owned = name.to_string();
        let completion = std::sync::Arc::new(super::context::Completion::<i32>::new());
        let comp = completion.clone();

        reactor_dispatch::send_to_reactor(move || {
            let bdev_name = std::ffi::CString::new(name_owned.as_str()).unwrap();
            unsafe {
                let bdev = ffi::spdk_bdev_get_by_name(bdev_name.as_ptr() as *const c_char);
                if bdev.is_null() {
                    comp.complete(0);
                    return;
                }
                ffi::spdk_bdev_unregister(bdev, Some(bdev_unregister_cb), comp.as_ptr());
            }
        });

        let rc = completion.wait();
        if rc != 0 {
            return Err(FrontendError::Bdev(format!(
                "spdk_bdev_unregister failed: rc={rc}"
            )));
        }

        self.bdevs.lock().unwrap().remove(name);
        Ok(())
    }
}

unsafe extern "C" fn bdev_unregister_cb(ctx: *mut std::os::raw::c_void, rc: i32) {
    let completion = super::context::Completion::<i32>::from_ptr(ctx);
    completion.complete(rc);
}
