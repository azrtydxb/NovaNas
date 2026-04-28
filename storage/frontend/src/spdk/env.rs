//! SPDK environment initialisation and reactor management.
//!
//! Ported from `storage/dataplane/src/spdk/env.rs`. The frontend runs
//! its own SPDK env in its own process, distinct from the data daemon's
//! — uses `--name novanas-frontend` and the same `/var/tmp/spdk.sock`
//! native RPC layout (the data daemon overrides its own socket path so
//! the two can coexist on the same host).

use crate::error::{FrontendError, Result};
use log::info;

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

/// Data passed to the SPDK startup callback via the arg pointer.
struct SpdkStartupData {
    listen_address: String,
    listen_port: u16,
}

/// Initialise the SPDK env and run the reactor. Blocks until
/// `shutdown_spdk_env()` is called (e.g. via signal handler).
///
/// `listen_port` is currently informational only; the NVMe-oF transport
/// listener is registered lazily by `crate::spdk_nvmf::SpdkNvmfTarget`
/// when the first BlockVolume arrives.
pub fn init_spdk_env(reactor_mask: &str, mem_size_mb: u32, listen_port: u16) -> Result<()> {
    info!(
        "frontend SPDK env init: reactor_mask={}, mem={}MB, listen_port={}",
        reactor_mask, mem_size_mb, listen_port
    );

    unsafe {
        let rust_size = std::mem::size_of::<ffi::spdk_app_opts>();
        info!("sizeof(spdk_app_opts) in Rust = {} bytes", rust_size);

        let mut opts: ffi::spdk_app_opts = std::mem::zeroed();
        ffi::spdk_app_opts_init(&mut opts, rust_size);

        let app_name = std::ffi::CString::new("novanas-frontend").unwrap();
        let reactor_mask_c = std::ffi::CString::new(reactor_mask).unwrap();
        let hugedir = std::ffi::CString::new("/dev/hugepages").unwrap();
        opts.name = app_name.as_ptr();
        opts.reactor_mask = reactor_mask_c.as_ptr();
        opts.mem_size = mem_size_mb as i32;
        opts.hugedir = hugedir.as_ptr();

        // Distinct DPDK file prefix so the frontend can coexist with the
        // data daemon on the same host (each SPDK process needs its own
        // hugepage file namespace). The data daemon uses the SPDK default
        // (`spdk_pid_<pid>`); we suffix `_frontend` to keep the two
        // processes' hugepage ranges separate.
        let pid = std::process::id();
        let file_prefix = std::ffi::CString::new(format!("spdk_pid_frontend_{}", pid)).unwrap();
        // SPDK reads file_prefix off opts.env_context — see lib/env_dpdk
        // for the documented "file-prefix=<x>" option syntax.
        let env_ctx =
            std::ffi::CString::new(format!("--file-prefix={}", file_prefix.to_str().unwrap()))
                .unwrap();
        opts.env_context = env_ctx.as_ptr() as *mut std::os::raw::c_char;

        // Native RPC socket. The frontend uses a distinct path so it
        // doesn't collide with the data daemon if both run on the same
        // host.
        let rpc_sock = std::ffi::CString::new("/var/tmp/spdk_frontend.sock").unwrap();
        opts.rpc_addr = rpc_sock.as_ptr();

        // Right-size iobuf pools for NVMe-oF TCP — same numbers the
        // dataplane uses (~296MB for the large-buffer pool).
        let mut iobuf_opts: ffi::spdk_iobuf_opts = std::mem::zeroed();
        ffi::spdk_iobuf_get_opts(&mut iobuf_opts, std::mem::size_of::<ffi::spdk_iobuf_opts>());
        info!(
            "iobuf defaults: small={}x{}B, large={}x{}B",
            iobuf_opts.small_pool_count,
            iobuf_opts.small_bufsize,
            iobuf_opts.large_pool_count,
            iobuf_opts.large_bufsize,
        );
        iobuf_opts.small_pool_count = 4096;
        iobuf_opts.large_pool_count = 2048;
        let rc = ffi::spdk_iobuf_set_opts(&iobuf_opts);
        info!(
            "spdk_iobuf_set_opts rc={} (small={}, large={})",
            rc, iobuf_opts.small_pool_count, iobuf_opts.large_pool_count
        );

        let startup_data = Box::new(SpdkStartupData {
            listen_address: String::new(),
            listen_port,
        });
        let arg = Box::into_raw(startup_data) as *mut std::os::raw::c_void;

        let rc = ffi::spdk_app_start(&mut opts, Some(spdk_startup_cb), arg);
        if rc != 0 {
            return Err(FrontendError::SpdkInit(format!(
                "spdk_app_start failed with rc={}",
                rc
            )));
        }
    }

    Ok(())
}

unsafe extern "C" fn spdk_startup_cb(arg: *mut std::os::raw::c_void) {
    info!("frontend SPDK startup callback: subsystems initialised");

    // Pre-allocate DMA buffer pool for I/O.
    super::reactor_dispatch::init_dma_pool();

    let data = Box::from_raw(arg as *mut SpdkStartupData);
    info!(
        "frontend SPDK startup data: listen_address='{}', listen_port={}",
        data.listen_address, data.listen_port
    );

    info!("frontend SPDK subsystems ready — reactor running");
}

pub fn shutdown_spdk_env() {
    unsafe {
        ffi::spdk_app_stop(0);
        ffi::spdk_app_fini();
    }
}
