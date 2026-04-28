//! Reactor dispatch — run SPDK operations on the reactor thread.
//!
//! Ported from `storage/dataplane/src/spdk/reactor_dispatch.rs`. SPDK
//! operations execute on the SPDK app thread; tonic / tokio threads
//! dispatch work via `spdk_thread_send_msg()` and either block on a
//! `Completion` or `.await` an `AsyncCompletion`.
//!
//! The frontend's hot path (custom volume bdev) submits I/O directly from
//! the reactor poll context, so the read/write helpers are kept here for
//! the bootstrap / debug surface only.

use crate::error::{FrontendError, Result};
use std::collections::HashMap;
use std::os::raw::c_char;
use std::sync::{Arc, Mutex, OnceLock};

use super::context::{AsyncCompletion, AsyncCompletionSender, Completion};

const DMA_POOL_SIZE: usize = 256;
const DMA_BUF_SIZE: usize = 64 * 1024;

struct DmaPool {
    buffers: Vec<*mut u8>,
}

// SAFETY: only accessed via send_to_reactor on the SPDK reactor thread, plus
// the lock around the pool.
unsafe impl Send for DmaPool {}

static DMA_POOL: OnceLock<Mutex<DmaPool>> = OnceLock::new();

/// Initialise the DMA buffer pool on the reactor thread.
pub fn init_dma_pool() {
    let mut buffers = Vec::with_capacity(DMA_POOL_SIZE);
    for _ in 0..DMA_POOL_SIZE {
        let buf =
            unsafe { ffi::spdk_dma_malloc(DMA_BUF_SIZE, 0x1000, std::ptr::null_mut()) as *mut u8 };
        if buf.is_null() {
            log::warn!(
                "DMA pool: only pre-allocated {} of {} buffers",
                buffers.len(),
                DMA_POOL_SIZE
            );
            break;
        }
        buffers.push(buf);
    }
    log::info!(
        "DMA buffer pool initialised: {} x {}KB",
        buffers.len(),
        DMA_BUF_SIZE / 1024
    );
    DMA_POOL.set(Mutex::new(DmaPool { buffers })).ok();
}

fn acquire_dma_buf(size: usize) -> *mut std::os::raw::c_void {
    if size <= DMA_BUF_SIZE {
        if let Some(pool) = DMA_POOL.get() {
            if let Ok(mut p) = pool.lock() {
                if let Some(buf) = p.buffers.pop() {
                    return buf as *mut std::os::raw::c_void;
                }
            }
        }
    }
    unsafe { ffi::spdk_dma_malloc(size, 0x1000, std::ptr::null_mut()) }
}

pub fn acquire_dma_buf_public(size: usize) -> *mut std::os::raw::c_void {
    acquire_dma_buf(size)
}

pub fn release_dma_buf_public(buf: *mut std::os::raw::c_void, size: usize) {
    release_dma_buf(buf, size);
}

fn release_dma_buf(buf: *mut std::os::raw::c_void, size: usize) {
    if size <= DMA_BUF_SIZE {
        if let Some(pool) = DMA_POOL.get() {
            if let Ok(mut p) = pool.lock() {
                if p.buffers.len() < DMA_POOL_SIZE {
                    p.buffers.push(buf as *mut u8);
                    return;
                }
            }
        }
    }
    unsafe {
        ffi::spdk_dma_free(buf);
    }
}

// ---------------------------------------------------------------------------
// Send-safe SPDK pointer wrapper
// ---------------------------------------------------------------------------

#[derive(Clone, Copy)]
pub struct SendPtr(usize);

unsafe impl Send for SendPtr {}

impl SendPtr {
    pub fn new(ptr: *mut std::os::raw::c_void) -> Self {
        Self(ptr as usize)
    }

    pub fn as_ptr(self) -> *mut std::os::raw::c_void {
        self.0 as *mut std::os::raw::c_void
    }
}

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
// Core dispatch primitives
// ---------------------------------------------------------------------------

/// Send a closure to the SPDK reactor thread (fire-and-forget).
pub fn send_to_reactor<F>(f: F)
where
    F: FnOnce() + Send + 'static,
{
    struct Ctx<F>(Option<F>);

    let ctx = Box::new(Ctx(Some(f)));
    let ctx_ptr = Box::into_raw(ctx) as *mut std::os::raw::c_void;

    unsafe extern "C" fn trampoline<F: FnOnce() + Send + 'static>(arg: *mut std::os::raw::c_void) {
        let mut ctx = Box::from_raw(arg as *mut Ctx<F>);
        if let Some(f) = ctx.0.take() {
            f();
        }
    }

    unsafe {
        let thread = ffi::spdk_thread_get_app_thread();
        assert!(!thread.is_null(), "SPDK app thread not available");
        let rc = ffi::spdk_thread_send_msg(thread, Some(trampoline::<F>), ctx_ptr);
        assert!(rc == 0, "spdk_thread_send_msg failed: rc={}", rc);
    }
}

/// Dispatch a synchronous operation to the reactor and wait for the result.
pub fn dispatch_sync<F, R>(f: F) -> R
where
    F: FnOnce() -> R + Send + 'static,
    R: Send + 'static,
{
    let on_spdk = unsafe { !ffi::spdk_get_thread().is_null() };
    if on_spdk {
        return f();
    }

    let completion = Arc::new(Completion::<R>::new());
    let comp = completion.clone();
    send_to_reactor(move || {
        let result = f();
        comp.complete(result);
    });
    completion.wait()
}

/// Dispatch a closure to the SPDK reactor and return a future for the result.
pub async fn dispatch_async<F, R>(f: F) -> R
where
    F: FnOnce() -> R + Send + 'static,
    R: Send + 'static,
{
    let on_spdk = unsafe { !ffi::spdk_get_thread().is_null() };
    if on_spdk {
        return f();
    }

    let (completion, mut sender) = AsyncCompletion::<R>::new();

    send_to_reactor(move || {
        let result = f();
        sender.complete(result);
    });

    completion.wait().await
}

// ---------------------------------------------------------------------------
// Bdev event callback (required non-null in SPDK v24.09)
// ---------------------------------------------------------------------------

unsafe extern "C" fn bdev_event_cb(
    _type_: ffi::spdk_bdev_event_type,
    _bdev: *mut ffi::spdk_bdev,
    _event_ctx: *mut std::os::raw::c_void,
) {
    // No-op — frontend tracks bdev lifecycle separately.
}

// ---------------------------------------------------------------------------
// Cached bdev descriptor + I/O channel
// ---------------------------------------------------------------------------

struct CachedBdevDesc {
    desc: *mut ffi::spdk_bdev_desc,
    block_size: u64,
}

unsafe impl Send for CachedBdevDesc {}
unsafe impl Sync for CachedBdevDesc {}

static BDEV_DESC_CACHE: OnceLock<Mutex<HashMap<String, CachedBdevDesc>>> = OnceLock::new();

fn bdev_desc_cache() -> &'static Mutex<HashMap<String, CachedBdevDesc>> {
    BDEV_DESC_CACHE.get_or_init(|| Mutex::new(HashMap::new()))
}

thread_local! {
    static CHANNEL_CACHE: std::cell::RefCell<HashMap<String, *mut ffi::spdk_io_channel>>
        = std::cell::RefCell::new(HashMap::new());
}

/// Close all cached bdev descriptors and channels. Call on shutdown.
pub fn close_all_cached_bdevs() {
    send_to_reactor(|| unsafe {
        CHANNEL_CACHE.with(|cache| {
            let mut cache = cache.borrow_mut();
            for (name, ch) in cache.drain() {
                log::info!("bdev channel cache: closing '{}'", name);
                ffi::spdk_put_io_channel(ch);
            }
        });
        let mut cache = bdev_desc_cache().lock().unwrap();
        for (name, entry) in cache.drain() {
            log::info!("bdev desc cache: closing '{}'", name);
            ffi::spdk_bdev_close(entry.desc);
        }
    });
}

// ---------------------------------------------------------------------------
// Malloc bdev helpers (used by tests / the frontend's debug helpers)
// ---------------------------------------------------------------------------

pub fn create_malloc_bdev(name: &str, num_blocks: u64, block_size: u32) -> Result<()> {
    let name = name.to_string();
    dispatch_sync(move || {
        let name_c = std::ffi::CString::new(name.as_str()).unwrap();
        unsafe {
            let mut opts: ffi::malloc_bdev_opts = std::mem::zeroed();
            opts.name = name_c.as_ptr() as *mut c_char;
            opts.num_blocks = num_blocks;
            opts.block_size = block_size;
            let mut bdev_ptr: *mut ffi::spdk_bdev = std::ptr::null_mut();
            let rc = ffi::create_malloc_disk(&mut bdev_ptr, &opts);
            if rc != 0 {
                Err(FrontendError::Bdev(format!(
                    "create_malloc_disk failed: rc={rc}"
                )))
            } else {
                Ok(())
            }
        }
    })
}

pub fn delete_malloc_bdev(name: &str) -> Result<()> {
    let name = name.to_string();
    let completion = Arc::new(Completion::<i32>::new());
    let comp = completion.clone();

    send_to_reactor(move || {
        let name_c = std::ffi::CString::new(name.as_str()).unwrap();

        unsafe extern "C" fn delete_done(cb_arg: *mut std::os::raw::c_void, rc: i32) {
            let comp = Arc::from_raw(cb_arg as *const Completion<i32>);
            comp.complete(rc);
        }

        unsafe {
            let bdev = ffi::spdk_bdev_get_by_name(name_c.as_ptr() as *const c_char);
            let comp_ptr = Arc::into_raw(comp) as *mut std::os::raw::c_void;
            if bdev.is_null() {
                let c = Arc::from_raw(comp_ptr as *const Completion<i32>);
                c.complete(0);
                return;
            }
            ffi::delete_malloc_disk(bdev, Some(delete_done), comp_ptr);
        }
    });

    let rc = completion.wait();
    if rc != 0 {
        Err(FrontendError::Bdev(format!(
            "delete_malloc_disk failed: rc={rc}"
        )))
    } else {
        Ok(())
    }
}

pub fn query_bdev(name: &str) -> Result<(u64, u32)> {
    let name = name.to_string();
    dispatch_sync(move || {
        let name_c = std::ffi::CString::new(name.as_str()).unwrap();
        unsafe {
            let bdev = ffi::spdk_bdev_get_by_name(name_c.as_ptr() as *const c_char);
            if bdev.is_null() {
                Err(FrontendError::Bdev(format!("bdev '{}' not found", name)))
            } else {
                let num_blocks = ffi::spdk_bdev_get_num_blocks(bdev);
                let block_size = ffi::spdk_bdev_get_block_size(bdev);
                Ok((num_blocks, block_size))
            }
        }
    })
}

// Suppress dead-code warnings on the AsyncCompletion bridge that's shipped for
// future I/O integration but not yet consumed.
#[allow(dead_code)]
fn _async_compat_guard(_s: AsyncCompletionSender<()>) {}
