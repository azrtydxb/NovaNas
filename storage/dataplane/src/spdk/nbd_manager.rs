//! NBD export manager — exposes SPDK bdevs as kernel `/dev/nbdN` devices.
//!
//! This is the integration point used by `DataplaneService.ExportMetadataVolumeNBD`.
//! The Go meta service calls that RPC at startup to obtain a `/dev/nbdN` path
//! backed by the chunk-engine metadata volume. It then mounts that block
//! device and opens BadgerDB on top of it — satisfying the "everything is
//! chunks" invariant (S11) for metadata storage.
//!
//! ## Flow
//!
//! 1. Caller supplies `(volume_name, size_bytes)` (derived from the RPC's
//!    locator: `(volume_name, root_chunk_id, volume_version)`).
//! 2. We ensure an SPDK bdev named `novanas_<volume_name>` exists, creating
//!    it via `novanas_bdev::create` if necessary. The chunk map for the
//!    volume is restored from the persistent metadata store on dataplane
//!    startup, so re-creating the bdev is stateless from the caller's POV.
//! 3. We pick the first free `/dev/nbdN` (N ∈ [0, 16)) by probing with
//!    `spdk_nbd_disk_find_by_nbd_path` on the reactor thread.
//! 4. We dispatch `spdk_nbd_start(bdev_name, nbd_path, cb, ctx)` to the
//!    reactor and block on a `Completion<i32>` until the callback fires.
//! 5. We store `(volume_name → (nbd_path, spdk_nbd_disk*))` in the
//!    manager's registry so `release_volume` can tear the export down.
//!
//! ## Kernel side
//!
//! The NBD kernel module must be loaded on the host with
//! `modprobe nbd nbds_max=16` — see `entrypoint.sh`.

use crate::error::{DataPlaneError, Result};
use std::collections::HashMap;
use std::sync::Mutex;

#[cfg(feature = "spdk-sys")]
use crate::spdk::context::Completion;
#[cfg(feature = "spdk-sys")]
use crate::spdk::reactor_dispatch;
#[cfg(feature = "spdk-sys")]
use std::sync::Arc;

#[cfg(feature = "spdk-sys")]
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

/// Upper bound on `/dev/nbdN` slots probed. Matches the default
/// `nbds_max=16` passed to `modprobe nbd`.
pub const MAX_NBD_SLOTS: u32 = 16;

/// Derive the SPDK bdev name from a volume name.
///
/// The novanas bdev module registers volumes under the `novanas_<name>`
/// namespace — see `crate::bdev::novanas_bdev::create`.
pub fn bdev_name_for_volume(volume_name: &str) -> String {
    format!("novanas_{}", volume_name)
}

/// Per-export bookkeeping entry.
#[derive(Debug)]
struct NbdExport {
    nbd_path: String,
    /// Opaque `*mut spdk_nbd_disk` pointer, stored as `usize` so the
    /// map is `Send + Sync`. Resolved back to a pointer on the reactor
    /// thread at release time.
    disk_ptr: usize,
}

/// Manages the lifecycle of NBD exports created via `spdk_nbd_start`.
///
/// Thread-safe: all public methods take `&self`. Bookkeeping is guarded
/// by a single `Mutex` — the SPDK-side work happens on the reactor thread
/// via `reactor_dispatch`.
pub struct NbdManager {
    /// `volume_name -> NbdExport`.
    exports: Mutex<HashMap<String, NbdExport>>,
}

impl Default for NbdManager {
    fn default() -> Self {
        Self::new()
    }
}

impl NbdManager {
    pub fn new() -> Self {
        Self {
            exports: Mutex::new(HashMap::new()),
        }
    }

    /// Return the nbd device path for a previously-exported volume, if any.
    pub fn get_path(&self, volume_name: &str) -> Option<String> {
        self.exports
            .lock()
            .unwrap()
            .get(volume_name)
            .map(|e| e.nbd_path.clone())
    }

    /// List all volume names currently exported. Primarily for diagnostics
    /// and tests.
    pub fn list(&self) -> Vec<String> {
        self.exports.lock().unwrap().keys().cloned().collect()
    }

    /// Return true if the given `/dev/nbdN` path is already in use by
    /// this manager. Used by path allocation to skip slots we own.
    fn path_is_tracked(&self, nbd_path: &str) -> bool {
        self.exports
            .lock()
            .unwrap()
            .values()
            .any(|e| e.nbd_path == nbd_path)
    }

    /// Pick the first `/dev/nbdN` slot in `[0, MAX_NBD_SLOTS)` that is
    /// not tracked locally **and** not already in use at the SPDK level
    /// (as reported by `probe`).
    ///
    /// `probe` receives the candidate path and returns true iff that
    /// path is free from SPDK's perspective. In production this is
    /// `spdk_nbd_disk_find_by_nbd_path(path).is_null()`; tests inject a
    /// pure function.
    fn pick_free_slot<F>(&self, probe: F) -> Result<String>
    where
        F: Fn(&str) -> bool,
    {
        for n in 0..MAX_NBD_SLOTS {
            let candidate = format!("/dev/nbd{}", n);
            if self.path_is_tracked(&candidate) {
                continue;
            }
            if probe(&candidate) {
                return Ok(candidate);
            }
        }
        Err(DataPlaneError::BdevError(format!(
            "no free /dev/nbdN slot in range [0, {})",
            MAX_NBD_SLOTS
        )))
    }

    /// Record a successful export. Extracted as a separate method so the
    /// bookkeeping is testable in isolation from the SPDK FFI path.
    fn record_export(&self, volume_name: &str, nbd_path: String, disk_ptr: usize) {
        self.exports
            .lock()
            .unwrap()
            .insert(volume_name.to_string(), NbdExport { nbd_path, disk_ptr });
    }

    /// Remove and return an export entry. Used by `release_volume` and
    /// failure-path cleanup.
    fn take_export(&self, volume_name: &str) -> Option<(String, usize)> {
        self.exports
            .lock()
            .unwrap()
            .remove(volume_name)
            .map(|e| (e.nbd_path, e.disk_ptr))
    }

    // ------------------------------------------------------------------
    // SPDK-backed paths (only compiled when spdk-sys is enabled).
    // ------------------------------------------------------------------

    /// Export `volume_name` (size `size_bytes`) as a kernel NBD device.
    /// Returns the chosen `/dev/nbdN` path.
    ///
    /// Idempotent: if the volume is already exported, the existing path
    /// is returned.
    #[cfg(feature = "spdk-sys")]
    pub fn export_volume(&self, volume_name: &str, size_bytes: u64) -> Result<String> {
        if let Some(path) = self.get_path(volume_name) {
            log::info!(
                "nbd_manager: volume '{}' already exported at {}",
                volume_name,
                path
            );
            return Ok(path);
        }

        // 1. Ensure the chunk-backed SPDK bdev exists.
        let bdev_name = bdev_name_for_volume(volume_name);
        ensure_bdev_registered(volume_name, size_bytes)?;

        // 2. Pick a free /dev/nbdN.
        let nbd_path = self.pick_free_slot(spdk_path_is_free)?;

        // 3. Start the export on the reactor.
        let disk_ptr = spdk_nbd_start_blocking(&bdev_name, &nbd_path).map_err(|e| {
            log::error!(
                "nbd_manager: spdk_nbd_start failed for volume '{}' at {}: {}",
                volume_name,
                nbd_path,
                e
            );
            e
        })?;

        // 4. Record bookkeeping. If this were to fail we'd leak the export;
        //    HashMap::insert cannot fail so this is infallible.
        self.record_export(volume_name, nbd_path.clone(), disk_ptr);

        log::info!(
            "nbd_manager: exported volume '{}' (bdev '{}') at {}",
            volume_name,
            bdev_name,
            nbd_path
        );
        Ok(nbd_path)
    }

    /// Tear down the NBD export for `volume_name`. Idempotent: returns
    /// `Ok(())` if the export is not present.
    #[cfg(feature = "spdk-sys")]
    pub fn release_volume(&self, volume_name: &str) -> Result<()> {
        let (nbd_path, disk_ptr) = match self.take_export(volume_name) {
            Some(e) => e,
            None => {
                log::info!(
                    "nbd_manager: release_volume('{}'): no active export, nothing to do",
                    volume_name
                );
                return Ok(());
            }
        };

        log::info!(
            "nbd_manager: releasing NBD export for volume '{}' at {}",
            volume_name,
            nbd_path
        );

        spdk_nbd_stop_blocking(disk_ptr)
    }
}

// ---------------------------------------------------------------------------
// SPDK FFI helpers (feature-gated)
// ---------------------------------------------------------------------------

/// Ensure an SPDK bdev named `novanas_<volume_name>` is registered. If
/// `novanas_bdev::create` has already registered this volume we return
/// without re-registering (idempotent — `bdev_registry` tracks this).
#[cfg(feature = "spdk-sys")]
fn ensure_bdev_registered(volume_name: &str, size_bytes: u64) -> Result<()> {
    // Fast path: already in the novanas bdev registry.
    {
        let registry = crate::bdev::novanas_bdev::bdev_registry().lock().unwrap();
        if registry.contains_key(volume_name) {
            return Ok(());
        }
    }

    log::info!(
        "nbd_manager: registering novanas bdev for volume '{}' ({} bytes)",
        volume_name,
        size_bytes
    );
    let _bdev_name = crate::bdev::novanas_bdev::create(volume_name, size_bytes)?;
    Ok(())
}

/// True iff SPDK reports that `nbd_path` is not currently exporting any
/// disk. Runs on the reactor thread.
#[cfg(feature = "spdk-sys")]
fn spdk_path_is_free(nbd_path: &str) -> bool {
    let path = nbd_path.to_string();
    reactor_dispatch::dispatch_sync(move || {
        let c_path = match std::ffi::CString::new(path.as_str()) {
            Ok(s) => s,
            Err(_) => return false,
        };
        let disk = unsafe { ffi::spdk_nbd_disk_find_by_nbd_path(c_path.as_ptr()) };
        disk.is_null()
    })
}

/// Call `spdk_nbd_start` on the reactor, block until the callback fires,
/// and return the resulting `spdk_nbd_disk*` as a `usize`.
#[cfg(feature = "spdk-sys")]
fn spdk_nbd_start_blocking(bdev_name: &str, nbd_path: &str) -> Result<usize> {
    /// Tuple of (rc, disk_ptr_as_usize) passed back through the callback.
    #[derive(Clone, Copy)]
    struct StartResult {
        rc: i32,
        disk: usize,
    }

    let completion: Arc<Completion<StartResult>> = Arc::new(Completion::new());

    // Callback: SPDK signature is
    //   void (*)(void *cb_arg, struct spdk_nbd_disk *nbd, int rc)
    unsafe extern "C" fn start_cb(
        cb_arg: *mut std::os::raw::c_void,
        nbd: *mut ffi::spdk_nbd_disk,
        rc: i32,
    ) {
        // Reconstitute the Arc we handed off via Arc::into_raw below.
        let comp = Arc::from_raw(cb_arg as *const Completion<StartResult>);
        comp.complete(StartResult {
            rc,
            disk: nbd as usize,
        });
    }

    let bdev_c = std::ffi::CString::new(bdev_name)
        .map_err(|e| DataPlaneError::BdevError(format!("bdev name contains NUL: {e}")))?;
    let path_c = std::ffi::CString::new(nbd_path)
        .map_err(|e| DataPlaneError::BdevError(format!("nbd path contains NUL: {e}")))?;

    let comp_for_dispatch = completion.clone();
    reactor_dispatch::send_to_reactor(move || unsafe {
        // Transfer ownership of one Arc strong-count into SPDK; the
        // callback will call Arc::from_raw to reclaim it.
        let raw = Arc::into_raw(comp_for_dispatch) as *mut std::os::raw::c_void;
        ffi::spdk_nbd_start(bdev_c.as_ptr(), path_c.as_ptr(), Some(start_cb), raw);
    });

    let result = completion.wait();
    if result.rc != 0 {
        return Err(DataPlaneError::BdevError(format!(
            "spdk_nbd_start('{}', '{}') failed: rc={}",
            bdev_name, nbd_path, result.rc
        )));
    }
    if result.disk == 0 {
        return Err(DataPlaneError::BdevError(format!(
            "spdk_nbd_start('{}', '{}') returned success but null disk pointer",
            bdev_name, nbd_path
        )));
    }
    Ok(result.disk)
}

/// Call `spdk_nbd_stop` on the reactor. `spdk_nbd_stop` is synchronous
/// (void return) so we just dispatch and wait for the closure to finish.
#[cfg(feature = "spdk-sys")]
fn spdk_nbd_stop_blocking(disk_ptr: usize) -> Result<()> {
    reactor_dispatch::dispatch_sync(move || unsafe {
        let disk = disk_ptr as *mut ffi::spdk_nbd_disk;
        if !disk.is_null() {
            ffi::spdk_nbd_stop(disk);
        }
    });
    Ok(())
}

// ---------------------------------------------------------------------------
// Tests — exercise bookkeeping + path allocation via injected probes.
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn bdev_name_for_volume_is_prefixed() {
        assert_eq!(bdev_name_for_volume("meta"), "novanas_meta");
        assert_eq!(bdev_name_for_volume("vol-0"), "novanas_vol-0");
    }

    #[test]
    fn pick_free_slot_skips_tracked_paths() {
        let mgr = NbdManager::new();
        // Pretend /dev/nbd0 is already exported by us.
        mgr.record_export("vol-a", "/dev/nbd0".to_string(), 0xdead);
        // Probe: only /dev/nbd1 is free at SPDK level.
        let got = mgr
            .pick_free_slot(|p| p == "/dev/nbd1")
            .expect("should find a slot");
        assert_eq!(got, "/dev/nbd1");
    }

    #[test]
    fn pick_free_slot_errors_when_all_occupied() {
        let mgr = NbdManager::new();
        // SPDK reports every slot busy.
        let err = mgr.pick_free_slot(|_| false).expect_err("should fail");
        let msg = format!("{err}");
        assert!(msg.contains("no free /dev/nbdN"), "unexpected error: {msg}");
    }

    #[test]
    fn pick_free_slot_skips_our_exports_even_if_spdk_says_free() {
        let mgr = NbdManager::new();
        // We own /dev/nbd0 and /dev/nbd1; SPDK says all free (stale view).
        mgr.record_export("a", "/dev/nbd0".to_string(), 1);
        mgr.record_export("b", "/dev/nbd1".to_string(), 2);
        let got = mgr.pick_free_slot(|_| true).unwrap();
        assert_eq!(got, "/dev/nbd2");
    }

    #[test]
    fn record_and_take_export_roundtrip() {
        let mgr = NbdManager::new();
        assert!(mgr.get_path("meta").is_none());
        mgr.record_export("meta", "/dev/nbd3".to_string(), 0xbeef);
        assert_eq!(mgr.get_path("meta"), Some("/dev/nbd3".to_string()));
        assert_eq!(mgr.list(), vec!["meta".to_string()]);
        let (path, ptr) = mgr.take_export("meta").unwrap();
        assert_eq!(path, "/dev/nbd3");
        assert_eq!(ptr, 0xbeef);
        assert!(mgr.get_path("meta").is_none());
        assert!(mgr.take_export("meta").is_none());
    }

    #[test]
    fn path_is_tracked_reflects_bookkeeping() {
        let mgr = NbdManager::new();
        assert!(!mgr.path_is_tracked("/dev/nbd7"));
        mgr.record_export("x", "/dev/nbd7".to_string(), 42);
        assert!(mgr.path_is_tracked("/dev/nbd7"));
        assert!(!mgr.path_is_tracked("/dev/nbd8"));
    }
}
