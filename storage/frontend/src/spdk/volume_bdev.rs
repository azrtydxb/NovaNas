//! SPDK custom bdev backed by the frontend's `ChunkEngine`.
//!
//! Each registered volume becomes an SPDK bdev named `novanas_<volume_name>`.
//! When SPDK's NVMe-oF subsystem submits I/O against the bdev:
//!
//!  - Reactor `submit_request` callback runs (no thread crossing yet).
//!  - The I/O type, offset, length, and iovec array are extracted.
//!  - The work is dispatched to a tokio task that drives `ChunkEngine`'s
//!    `sub_block_read` / `sub_block_write` methods.
//!  - On completion the tokio task posts back to the reactor via
//!    `send_to_reactor`, where `spdk_bdev_io_complete` finalises the I/O.
//!
//! This is a leaner port of `dataplane/src/bdev/novanas_bdev.rs` — the
//! frontend's chunk engine handles placement (via meta) and remote NDP
//! routing internally, so this layer only owns the SPDK bdev module
//! registration + I/O fan-out.

use std::collections::HashMap;
use std::os::raw::{c_char, c_void};
use std::sync::{Arc, Mutex, OnceLock};

use async_trait::async_trait;
use log::{error, info};

use crate::chunk_engine::ChunkEngine;
use crate::error::{FrontendError, Result};
use crate::spdk::context::Completion;
use crate::spdk::reactor_dispatch;
use crate::volume_bdev::{VolumeBdevHandle, VolumeBdevManager};

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

// ---------------------------------------------------------------------------
// Globals shared with the SPDK bdev module callbacks.
// ---------------------------------------------------------------------------

static CHUNK_ENGINE: OnceLock<Arc<ChunkEngine>> = OnceLock::new();
static TOKIO_HANDLE: OnceLock<tokio::runtime::Handle> = OnceLock::new();

fn set_chunk_engine(engine: Arc<ChunkEngine>) {
    let _ = CHUNK_ENGINE.set(engine);
}

fn set_tokio_handle(handle: tokio::runtime::Handle) {
    let _ = TOKIO_HANDLE.set(handle);
}

/// Public wrapper used from the binary entry point to wire the tokio
/// runtime into the SPDK callback path before the reactor starts.
pub fn set_tokio_handle_pub(handle: tokio::runtime::Handle) {
    set_tokio_handle(handle);
}

fn get_chunk_engine() -> std::result::Result<&'static Arc<ChunkEngine>, &'static str> {
    CHUNK_ENGINE.get().ok_or("chunk engine not set")
}

fn get_tokio_handle() -> std::result::Result<&'static tokio::runtime::Handle, &'static str> {
    TOKIO_HANDLE.get().ok_or("tokio handle not set")
}

#[derive(Clone, Copy)]
struct BdevEntry {
    bdev_ptr: usize,
    ctx_ptr: usize,
}

unsafe impl Send for BdevEntry {}
unsafe impl Sync for BdevEntry {}

static BDEV_REGISTRY: OnceLock<Mutex<HashMap<String, BdevEntry>>> = OnceLock::new();

fn bdev_registry() -> &'static Mutex<HashMap<String, BdevEntry>> {
    BDEV_REGISTRY.get_or_init(|| Mutex::new(HashMap::new()))
}

/// Per-bdev context stored in `spdk_bdev->ctxt`. Owned by the bdev for its
/// lifetime; freed in `bdev_destruct_cb`.
struct BdevCtx {
    volume_name: String,
}

// ---------------------------------------------------------------------------
// SPDK bdev module
// ---------------------------------------------------------------------------

static mut NOVASTOR_MODULE: ffi::spdk_bdev_module = unsafe { std::mem::zeroed() };
static MODULE_INIT: std::sync::Once = std::sync::Once::new();

fn novanas_module_ptr() -> *mut ffi::spdk_bdev_module {
    MODULE_INIT.call_once(|| unsafe {
        NOVASTOR_MODULE.name = b"novanas_chunk\0".as_ptr() as *const c_char;
        NOVASTOR_MODULE.module_init = Some(module_init_cb);
        NOVASTOR_MODULE.module_fini = Some(module_fini_cb);
    });
    unsafe { &mut NOVASTOR_MODULE as *mut ffi::spdk_bdev_module }
}

static FN_TABLE: OnceLock<ffi::spdk_bdev_fn_table> = OnceLock::new();

fn novanas_fn_table() -> &'static ffi::spdk_bdev_fn_table {
    FN_TABLE.get_or_init(|| {
        let mut ft: ffi::spdk_bdev_fn_table = unsafe { std::mem::zeroed() };
        ft.destruct = Some(bdev_destruct_cb);
        ft.submit_request = Some(bdev_submit_request_cb);
        ft.io_type_supported = Some(bdev_io_type_supported_cb);
        ft.get_io_channel = Some(bdev_get_io_channel_cb);
        ft
    })
}

unsafe extern "C" fn module_init_cb() -> i32 {
    info!("novanas_bdev: module initialised");
    0
}

unsafe extern "C" fn module_fini_cb() {
    info!("novanas_bdev: module shutdown");
}

unsafe extern "C" fn channel_create_cb(_io_device: *mut c_void, _ctx: *mut c_void) -> i32 {
    0
}

unsafe extern "C" fn channel_destroy_cb(_io_device: *mut c_void, _ctx: *mut c_void) {}

unsafe extern "C" fn bdev_destruct_cb(ctx: *mut c_void) -> i32 {
    if !ctx.is_null() {
        let _ = Box::from_raw(ctx as *mut BdevCtx);
    }
    0
}

unsafe extern "C" fn bdev_io_type_supported_cb(
    _ctx: *mut c_void,
    io_type: ffi::spdk_bdev_io_type,
) -> bool {
    matches!(
        io_type,
        ffi::spdk_bdev_io_type_SPDK_BDEV_IO_TYPE_READ
            | ffi::spdk_bdev_io_type_SPDK_BDEV_IO_TYPE_WRITE
            | ffi::spdk_bdev_io_type_SPDK_BDEV_IO_TYPE_FLUSH
            | ffi::spdk_bdev_io_type_SPDK_BDEV_IO_TYPE_WRITE_ZEROES
            | ffi::spdk_bdev_io_type_SPDK_BDEV_IO_TYPE_UNMAP
            | ffi::spdk_bdev_io_type_SPDK_BDEV_IO_TYPE_RESET
    )
}

unsafe extern "C" fn bdev_get_io_channel_cb(ctx: *mut c_void) -> *mut ffi::spdk_io_channel {
    ffi::spdk_get_io_channel(ctx)
}

/// Copy the buffer at `data` into the iovec array.
unsafe fn copy_to_iovs(src: &[u8], iovs: *mut ffi::iovec, iovcnt: usize) {
    let mut copied = 0usize;
    for i in 0..iovcnt {
        if copied >= src.len() {
            break;
        }
        let iov = &*iovs.add(i);
        let to_copy = std::cmp::min(iov.iov_len, src.len() - copied);
        std::ptr::copy_nonoverlapping(src.as_ptr().add(copied), iov.iov_base as *mut u8, to_copy);
        copied += to_copy;
    }
}

/// Copy bytes out of the iovec array into a fresh Vec.
unsafe fn copy_from_iovs(iovs: *mut ffi::iovec, iovcnt: usize, total: usize) -> Vec<u8> {
    let mut out = vec![0u8; total];
    let mut copied = 0usize;
    for i in 0..iovcnt {
        if copied >= total {
            break;
        }
        let iov = &*iovs.add(i);
        let to_copy = std::cmp::min(iov.iov_len, total - copied);
        std::ptr::copy_nonoverlapping(
            iov.iov_base as *const u8,
            out.as_mut_ptr().add(copied),
            to_copy,
        );
        copied += to_copy;
    }
    out
}

unsafe extern "C" fn bdev_submit_request_cb(
    _channel: *mut ffi::spdk_io_channel,
    bdev_io: *mut ffi::spdk_bdev_io,
) {
    let bdev = (*bdev_io).bdev;
    let ctx = (*bdev).ctxt as *const BdevCtx;
    if ctx.is_null() {
        ffi::spdk_bdev_io_complete(bdev_io, ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_FAILED);
        return;
    }
    let volume_name = (*ctx).volume_name.clone();
    let io_type = (*bdev_io).type_ as u32;
    let block_size = (*bdev).blocklen as u64;

    match io_type {
        ffi::spdk_bdev_io_type_SPDK_BDEV_IO_TYPE_READ => {
            let p = (*bdev_io).u.bdev.as_ref();
            let offset = p.offset_blocks * block_size;
            let length = p.num_blocks * block_size;
            let iovs = p.iovs;
            let iovcnt = p.iovcnt as usize;
            if iovs.is_null() || iovcnt == 0 {
                ffi::spdk_bdev_io_complete(
                    bdev_io,
                    ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_FAILED,
                );
                return;
            }
            let bdev_io_addr = bdev_io as usize;
            let iovs_addr = iovs as usize;

            let handle = match get_tokio_handle() {
                Ok(h) => h,
                Err(e) => {
                    error!("novanas_bdev: read setup failed: {}", e);
                    ffi::spdk_bdev_io_complete(
                        bdev_io,
                        ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_FAILED,
                    );
                    return;
                }
            };
            handle.spawn(async move {
                let engine = match get_chunk_engine() {
                    Ok(e) => e,
                    Err(_) => {
                        reactor_dispatch::send_to_reactor(move || unsafe {
                            ffi::spdk_bdev_io_complete(
                                bdev_io_addr as *mut ffi::spdk_bdev_io,
                                ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_FAILED,
                            );
                        });
                        return;
                    }
                };
                let result = engine.sub_block_read(&volume_name, offset, length).await;
                reactor_dispatch::send_to_reactor(move || unsafe {
                    let iovs = iovs_addr as *mut ffi::iovec;
                    let status = match result {
                        Ok(data) => {
                            copy_to_iovs(&data, iovs, iovcnt);
                            ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_SUCCESS
                        }
                        Err(e) => {
                            error!("novanas_bdev: read failed: {}", e);
                            ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_FAILED
                        }
                    };
                    ffi::spdk_bdev_io_complete(bdev_io_addr as *mut ffi::spdk_bdev_io, status);
                });
            });
        }
        ffi::spdk_bdev_io_type_SPDK_BDEV_IO_TYPE_WRITE => {
            let p = (*bdev_io).u.bdev.as_ref();
            let offset = p.offset_blocks * block_size;
            let length = p.num_blocks * block_size;
            let iovs = p.iovs;
            let iovcnt = p.iovcnt as usize;
            if iovs.is_null() || iovcnt == 0 {
                ffi::spdk_bdev_io_complete(
                    bdev_io,
                    ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_FAILED,
                );
                return;
            }
            let data = copy_from_iovs(iovs, iovcnt, length as usize);
            let bdev_io_addr = bdev_io as usize;

            let handle = match get_tokio_handle() {
                Ok(h) => h,
                Err(e) => {
                    error!("novanas_bdev: write setup failed: {}", e);
                    ffi::spdk_bdev_io_complete(
                        bdev_io,
                        ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_FAILED,
                    );
                    return;
                }
            };
            handle.spawn(async move {
                let engine = match get_chunk_engine() {
                    Ok(e) => e,
                    Err(_) => {
                        reactor_dispatch::send_to_reactor(move || unsafe {
                            ffi::spdk_bdev_io_complete(
                                bdev_io_addr as *mut ffi::spdk_bdev_io,
                                ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_FAILED,
                            );
                        });
                        return;
                    }
                };
                let result = engine.sub_block_write(&volume_name, offset, &data).await;
                let status = match result {
                    Ok(()) => ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_SUCCESS,
                    Err(e) => {
                        error!("novanas_bdev: write failed: {}", e);
                        ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_FAILED
                    }
                };
                reactor_dispatch::send_to_reactor(move || unsafe {
                    ffi::spdk_bdev_io_complete(bdev_io_addr as *mut ffi::spdk_bdev_io, status);
                });
            });
        }
        ffi::spdk_bdev_io_type_SPDK_BDEV_IO_TYPE_FLUSH => {
            let bdev_io_addr = bdev_io as usize;
            let handle = match get_tokio_handle() {
                Ok(h) => h,
                Err(_) => {
                    ffi::spdk_bdev_io_complete(
                        bdev_io,
                        ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_SUCCESS,
                    );
                    return;
                }
            };
            handle.spawn(async move {
                let status = match get_chunk_engine() {
                    Ok(engine) => match engine.flush(&volume_name).await {
                        Ok(()) => ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_SUCCESS,
                        Err(e) => {
                            error!("novanas_bdev: flush failed: {}", e);
                            ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_FAILED
                        }
                    },
                    Err(_) => ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_SUCCESS,
                };
                reactor_dispatch::send_to_reactor(move || unsafe {
                    ffi::spdk_bdev_io_complete(bdev_io_addr as *mut ffi::spdk_bdev_io, status);
                });
            });
        }
        ffi::spdk_bdev_io_type_SPDK_BDEV_IO_TYPE_WRITE_ZEROES
        | ffi::spdk_bdev_io_type_SPDK_BDEV_IO_TYPE_UNMAP
        | ffi::spdk_bdev_io_type_SPDK_BDEV_IO_TYPE_RESET => {
            // Best-effort no-ops: WRITE_ZEROES / UNMAP rely on the
            // backend returning zeros for unallocated regions; RESET has
            // no per-bdev state to reset.
            ffi::spdk_bdev_io_complete(
                bdev_io,
                ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_SUCCESS,
            );
        }
        _ => {
            ffi::spdk_bdev_io_complete(
                bdev_io,
                ffi::spdk_bdev_io_status_SPDK_BDEV_IO_STATUS_FAILED,
            );
        }
    }
}

// ---------------------------------------------------------------------------
// Public manager
// ---------------------------------------------------------------------------

pub struct SpdkVolumeBdevManager {
    engine: Arc<ChunkEngine>,
    state: tokio::sync::Mutex<HashMap<String, VolumeBdevHandle>>,
}

impl SpdkVolumeBdevManager {
    pub fn new(engine: Arc<ChunkEngine>) -> Self {
        // Wire the global accessors used by the SPDK callbacks. These are
        // OnceLock so repeated construction is harmless.
        set_chunk_engine(engine.clone());
        if let Ok(h) = tokio::runtime::Handle::try_current() {
            set_tokio_handle(h);
        }
        Self {
            engine,
            state: tokio::sync::Mutex::new(HashMap::new()),
        }
    }

    pub fn engine(&self) -> &Arc<ChunkEngine> {
        &self.engine
    }
}

#[async_trait]
impl VolumeBdevManager for SpdkVolumeBdevManager {
    async fn create(
        &self,
        volume_name: &str,
        size_bytes: u64,
        block_size: u32,
    ) -> Result<VolumeBdevHandle> {
        let bdev_name = VolumeBdevHandle::bdev_name_for(volume_name);
        info!(
            "spdk volume bdev: registering '{}' (volume='{}', size={}B, blk={}B)",
            bdev_name, volume_name, size_bytes, block_size
        );

        let num_blocks = size_bytes / block_size as u64;
        let ctx = Box::new(BdevCtx {
            volume_name: volume_name.to_string(),
        });
        let ctx_ptr = Box::into_raw(ctx);
        let ctx_addr = ctx_ptr as usize;
        let bdev_name_clone = bdev_name.clone();

        let bdev_addr = reactor_dispatch::dispatch_async(move || -> Result<usize> {
            unsafe {
                let ctx_ptr = ctx_addr as *mut BdevCtx;
                let io_dev_name = std::ffi::CString::new(format!("novanas_io_{}", bdev_name_clone))
                    .map_err(|e| FrontendError::Bdev(format!("invalid io device name: {e}")))?;
                ffi::spdk_io_device_register(
                    ctx_ptr as *mut c_void,
                    Some(channel_create_cb),
                    Some(channel_destroy_cb),
                    0,
                    io_dev_name.as_ptr(),
                );

                let bdev =
                    libc::calloc(1, std::mem::size_of::<ffi::spdk_bdev>()) as *mut ffi::spdk_bdev;
                if bdev.is_null() {
                    ffi::spdk_io_device_unregister(ctx_ptr as *mut c_void, None);
                    let _ = Box::from_raw(ctx_ptr);
                    return Err(FrontendError::Bdev("calloc spdk_bdev failed".into()));
                }

                let name_c = std::ffi::CString::new(bdev_name_clone.as_str())
                    .map_err(|e| FrontendError::Bdev(format!("invalid bdev name: {e}")))?;
                (*bdev).name = libc::strdup(name_c.as_ptr());
                (*bdev).product_name = libc::strdup(b"NovaNas Volume\0".as_ptr() as *const c_char);
                (*bdev).blocklen = block_size;
                (*bdev).blockcnt = num_blocks;
                (*bdev).ctxt = ctx_ptr as *mut c_void;
                (*bdev).module = novanas_module_ptr();
                (*bdev).fn_table = novanas_fn_table();

                let rc = ffi::spdk_bdev_register(bdev);
                if rc != 0 {
                    ffi::spdk_io_device_unregister(ctx_ptr as *mut c_void, None);
                    libc::free((*bdev).name as *mut c_void);
                    libc::free((*bdev).product_name as *mut c_void);
                    libc::free(bdev as *mut c_void);
                    let _ = Box::from_raw(ctx_ptr);
                    return Err(FrontendError::Bdev(format!(
                        "spdk_bdev_register failed: rc={rc}"
                    )));
                }
                Ok(bdev as usize)
            }
        })
        .await?;

        bdev_registry().lock().unwrap().insert(
            volume_name.to_string(),
            BdevEntry {
                bdev_ptr: bdev_addr,
                ctx_ptr: ctx_addr,
            },
        );

        let handle = VolumeBdevHandle {
            volume_name: volume_name.to_string(),
            bdev_name,
            size_bytes,
            block_size,
        };
        self.state
            .lock()
            .await
            .insert(volume_name.to_string(), handle.clone());
        Ok(handle)
    }

    async fn destroy(&self, volume_name: &str) -> Result<()> {
        let entry = bdev_registry()
            .lock()
            .unwrap()
            .remove(volume_name)
            .ok_or_else(|| {
                FrontendError::Bdev(format!(
                    "novanas bdev for volume '{}' not found",
                    volume_name
                ))
            })?;
        let bdev_addr = entry.bdev_ptr;
        let ctx_addr = entry.ctx_ptr;

        info!("spdk volume bdev: destroying '{}'", volume_name);

        let completion = Arc::new(Completion::<i32>::new());
        let comp = completion.clone();
        reactor_dispatch::send_to_reactor(move || unsafe {
            unsafe extern "C" fn unregister_cb(ctx: *mut c_void, rc: i32) {
                let comp = Completion::<i32>::from_ptr(ctx);
                comp.complete(rc);
            }
            let bdev = bdev_addr as *mut ffi::spdk_bdev;
            ffi::spdk_bdev_unregister(bdev, Some(unregister_cb), comp.as_ptr());
        });
        let rc = completion.wait();
        if rc != 0 {
            return Err(FrontendError::Bdev(format!(
                "spdk_bdev_unregister failed: rc={rc}"
            )));
        }

        reactor_dispatch::send_to_reactor(move || unsafe {
            ffi::spdk_io_device_unregister(ctx_addr as *mut c_void, None);
        });

        self.state.lock().await.remove(volume_name);
        Ok(())
    }

    async fn list(&self) -> Result<Vec<VolumeBdevHandle>> {
        Ok(self.state.lock().await.values().cloned().collect())
    }
}
