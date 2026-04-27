//! SPDK environment initialization and reactor management.

use crate::config::DataPlaneConfig;
use crate::error::{DataPlaneError, Result};
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
///
/// Architecture-v2: NVMe-oF target spin-up moved to the frontend daemon and
/// the management gRPC server is now started from `main.rs` outside the
/// SPDK reactor; nothing the startup callback needs is variable today, but
/// the indirection is preserved so future poller initialisation remains a
/// drop-in.
struct SpdkStartupData {}

pub fn init_spdk_env(config: &DataPlaneConfig) -> Result<()> {
    info!(
        "SPDK env init: reactor_mask={}, mem={}MB",
        config.reactor_mask, config.mem_size
    );

    unsafe {
        let rust_size = std::mem::size_of::<ffi::spdk_app_opts>();
        info!("sizeof(spdk_app_opts) in Rust = {} bytes", rust_size);

        let mut opts: ffi::spdk_app_opts = std::mem::zeroed();
        ffi::spdk_app_opts_init(&mut opts, rust_size);

        let app_name = std::ffi::CString::new("novanas-dataplane").unwrap();
        let reactor_mask = std::ffi::CString::new(config.reactor_mask.as_str()).unwrap();
        let hugedir = std::ffi::CString::new("/dev/hugepages").unwrap();
        opts.name = app_name.as_ptr();
        opts.reactor_mask = reactor_mask.as_ptr();
        opts.mem_size = config.mem_size as i32;
        opts.hugedir = hugedir.as_ptr();
        // Enable SPDK's native RPC socket for SPDK-internal operations only.
        // This is used by the Rust dataplane to call SPDK subsystem methods
        // (e.g. bdev_nvme_attach_controller for NVMe-oF initiator). The Go
        // agent NEVER connects to this socket — all Go→Rust communication
        // uses gRPC exclusively (invariant #5).
        let rpc_sock = std::ffi::CString::new("/var/tmp/spdk.sock").unwrap();
        opts.rpc_addr = rpc_sock.as_ptr();

        // Right-size iobuf pools for NVMe-oF TCP transport.
        // NVMe-oF TCP needs ~383 large buffers per poll group. Use 2048 to
        // ensure the pool never starves — insufficient buffers cause I/O hangs.
        // small=4096*8K=32MB, large=2048*132K=264MB → ~296MB total.
        let mut iobuf_opts: ffi::spdk_iobuf_opts = std::mem::zeroed();
        ffi::spdk_iobuf_get_opts(&mut iobuf_opts, std::mem::size_of::<ffi::spdk_iobuf_opts>());
        info!(
            "iobuf defaults from get_opts: small={}x{}B, large={}x{}B, opts_size={}",
            iobuf_opts.small_pool_count,
            iobuf_opts.small_bufsize,
            iobuf_opts.large_pool_count,
            iobuf_opts.large_bufsize,
            std::mem::size_of_val(&iobuf_opts),
        );
        iobuf_opts.small_pool_count = 4096;
        iobuf_opts.large_pool_count = 2048;
        let rc = ffi::spdk_iobuf_set_opts(&iobuf_opts);
        info!(
            "spdk_iobuf_set_opts rc={}, requested: small={}, large={}",
            rc, iobuf_opts.small_pool_count, iobuf_opts.large_pool_count
        );

        // Package config for the startup callback. The callback runs
        // inside spdk_app_start *before* the reactor loop blocks, so SPDK
        // subsystems (bdev, etc.) are ready before higher-level services
        // touch the bdev layer. NVMe-oF target wiring used to live in
        // this callback; it has moved to the frontend daemon.
        let _ = config;
        let startup_data = Box::new(SpdkStartupData {});
        let arg = Box::into_raw(startup_data) as *mut std::os::raw::c_void;

        let rc = ffi::spdk_app_start(&mut opts, Some(spdk_startup_cb), arg);
        if rc != 0 {
            return Err(DataPlaneError::SpdkInit(format!(
                "spdk_app_start failed with rc={}",
                rc
            )));
        }
    }

    Ok(())
}

unsafe extern "C" fn spdk_startup_cb(arg: *mut std::os::raw::c_void) {
    info!("SPDK startup callback: subsystems initialized");

    // Pre-allocate DMA buffer pool for bdev I/O (eliminates per-I/O malloc).
    super::reactor_dispatch::init_dma_pool();

    // Reactor-native NDP client — 50μs poller, partial write buffering.
    #[cfg(feature = "spdk-sys")]
    crate::chunk::reactor_ndp::init();

    // Recover the startup data passed through the arg pointer (currently
    // empty — preserved as a drop-in for future poller wiring).
    let _data = Box::from_raw(arg as *mut SpdkStartupData);

    info!("SPDK subsystems ready");
}

pub fn shutdown_spdk_env() {
    unsafe {
        ffi::spdk_app_stop(0);
        ffi::spdk_app_fini();
    }
}
