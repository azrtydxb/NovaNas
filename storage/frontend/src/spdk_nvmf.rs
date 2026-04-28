//! SPDK-backed `NvmfTarget` implementation.
//!
//! Ported from the now-removed `dataplane/src/spdk/nvmf_manager.rs` (kept
//! at architecture-v2 commit `3e5a1fb^`). The frontend owns its own
//! NVMe-oF target and TCP transport — no shared state with the data
//! daemon. Each `BlockVolume` becomes one subsystem with NQN
//! `nqn.2024-01.io.novanas:volume-<name>` and a single namespace pointing
//! at the volume's `novanas_<volume>` bdev.
//!
//! All FFI calls are dispatched to the SPDK reactor thread via
//! `reactor_dispatch::dispatch_sync` / `send_to_reactor`. Async SPDK
//! operations (add_listener, subsystem_start/stop/destroy) chain through
//! callbacks that signal a `Completion`.

use std::collections::HashMap;
use std::os::raw::{c_char, c_void};
use std::sync::{Arc, Mutex, Once};

use async_trait::async_trait;
use log::{error, info, warn};

use crate::error::{FrontendError, Result};
use crate::nvmf::{NvmfTarget, SubsystemSpec};
use crate::spdk::context::Completion;
use crate::spdk::reactor_dispatch::{self, SendPtr};

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

const EINPROGRESS: i32 = -115;
const DELETE_RETRY_US: u64 = 50_000;

struct TargetCreateCtx {
    subsystem: SendPtr,
    trid: Box<ffi::spdk_nvme_transport_id>,
    completion: Arc<Completion<i32>>,
}

unsafe impl Send for TargetCreateCtx {}

struct TargetDeleteCtx {
    subsystem: SendPtr,
    completion: Arc<Completion<i32>>,
}

unsafe impl Send for TargetDeleteCtx {}

pub struct SpdkNvmfTarget {
    listen_address: String,
    listen_port: u16,
    subsystems: Mutex<HashMap<String, SubsystemSpec>>,
    tgt_ptr: Mutex<Option<SendPtr>>,
    transport_once: Once,
    transport_err: Mutex<Option<String>>,
}

impl SpdkNvmfTarget {
    pub fn new(listen_address: impl Into<String>, listen_port: u16) -> Self {
        Self {
            listen_address: listen_address.into(),
            listen_port,
            subsystems: Mutex::new(HashMap::new()),
            tgt_ptr: Mutex::new(None),
            transport_once: Once::new(),
            transport_err: Mutex::new(None),
        }
    }

    pub fn listen_address(&self) -> &str {
        &self.listen_address
    }

    pub fn listen_port(&self) -> u16 {
        self.listen_port
    }

    fn ensure_transport(&self) -> Result<()> {
        self.transport_once.call_once(|| {
            if let Err(e) = self.init_transport() {
                *self.transport_err.lock().unwrap() = Some(e.to_string());
            }
        });
        if let Some(ref err_msg) = *self.transport_err.lock().unwrap() {
            return Err(FrontendError::Nvmf(format!(
                "transport init failed: {err_msg}"
            )));
        }
        Ok(())
    }

    fn init_transport(&self) -> Result<()> {
        info!("frontend: initialising NVMe-oF TCP transport");

        let (tgt_send, transport_send) =
            reactor_dispatch::dispatch_sync(|| -> Result<(SendPtr, SendPtr)> {
                unsafe {
                    let mut tgt = ffi::spdk_nvmf_get_first_tgt();
                    if tgt.is_null() {
                        let mut opts: ffi::spdk_nvmf_target_opts = std::mem::zeroed();
                        tgt = ffi::spdk_nvmf_tgt_create(&mut opts);
                        if tgt.is_null() {
                            return Err(FrontendError::Nvmf("spdk_nvmf_tgt_create failed".into()));
                        }
                        info!("frontend: created new NVMe-oF target");
                    }

                    let transport_name = std::ffi::CString::new("TCP").unwrap();
                    let mut transport_opts: ffi::spdk_nvmf_transport_opts = std::mem::zeroed();
                    let ok = ffi::spdk_nvmf_transport_opts_init(
                        transport_name.as_ptr() as *const c_char,
                        &mut transport_opts,
                        std::mem::size_of::<ffi::spdk_nvmf_transport_opts>(),
                    );
                    if !ok {
                        return Err(FrontendError::Nvmf(
                            "spdk_nvmf_transport_opts_init failed for TCP".into(),
                        ));
                    }

                    // Tune transport: max_qpairs_per_ctrlr=8, io_unit_size=128KB.
                    // Field offsets match SPDK v24.09's spdk_nvmf_transport_opts.
                    let opts_ptr = &mut transport_opts as *mut _ as *mut u8;
                    *(opts_ptr.add(2) as *mut u16) = 8;
                    *(opts_ptr.add(12) as *mut u32) = 131072;
                    info!("frontend: NVMe-oF TCP opts: max_qpairs_per_ctrlr=8, io_unit_size=128KB");

                    let transport = ffi::spdk_nvmf_transport_create(
                        transport_name.as_ptr() as *const c_char,
                        &mut transport_opts,
                    );
                    if transport.is_null() {
                        return Err(FrontendError::Nvmf(
                            "spdk_nvmf_transport_create failed for TCP".into(),
                        ));
                    }
                    Ok((
                        SendPtr::new(tgt as *mut c_void),
                        SendPtr::new(transport as *mut c_void),
                    ))
                }
            })?;

        let completion = Arc::new(Completion::<i32>::new());
        let comp = completion.clone();
        reactor_dispatch::send_to_reactor(move || {
            unsafe extern "C" fn add_transport_done(cb_arg: *mut c_void, status: i32) {
                let comp = Arc::from_raw(cb_arg as *const Completion<i32>);
                comp.complete(status);
            }
            unsafe {
                let comp_ptr = Arc::into_raw(comp) as *mut c_void;
                ffi::spdk_nvmf_tgt_add_transport(
                    tgt_send.as_ptr() as *mut ffi::spdk_nvmf_tgt,
                    transport_send.as_ptr() as *mut ffi::spdk_nvmf_transport,
                    Some(add_transport_done),
                    comp_ptr,
                );
            }
        });
        let rc = completion.wait();
        if rc != 0 {
            return Err(FrontendError::Nvmf(format!(
                "spdk_nvmf_tgt_add_transport failed: rc={rc}"
            )));
        }

        // Create the poll group on the reactor — without it SPDK never
        // accepts incoming TCP connections.
        let tgt_send_pg = tgt_send;
        let _pg = reactor_dispatch::dispatch_sync(move || -> Result<SendPtr> {
            unsafe {
                let tgt = tgt_send_pg.as_ptr() as *mut ffi::spdk_nvmf_tgt;
                let poll_group = ffi::spdk_nvmf_poll_group_create(tgt);
                if poll_group.is_null() {
                    return Err(FrontendError::Nvmf(
                        "spdk_nvmf_poll_group_create failed".into(),
                    ));
                }
                Ok(SendPtr::new(poll_group as *mut c_void))
            }
        })?;

        *self.tgt_ptr.lock().unwrap() = Some(tgt_send);
        info!("frontend: NVMe-oF TCP transport ready");
        Ok(())
    }

    fn get_tgt_ptr(&self) -> Result<SendPtr> {
        if let Some(p) = *self.tgt_ptr.lock().unwrap() {
            return Ok(p);
        }
        let p = reactor_dispatch::dispatch_sync(|| -> Result<SendPtr> {
            unsafe {
                let tgt = ffi::spdk_nvmf_get_first_tgt();
                if tgt.is_null() {
                    Err(FrontendError::Nvmf("no NVMe-oF target available".into()))
                } else {
                    Ok(SendPtr::new(tgt as *mut c_void))
                }
            }
        })?;
        *self.tgt_ptr.lock().unwrap() = Some(p);
        Ok(p)
    }

    fn create_target(&self, spec: &SubsystemSpec) -> Result<()> {
        self.ensure_transport()?;
        let nqn = spec.nqn();
        info!(
            "frontend: creating NVMe-oF subsystem nqn={}, bdev={}, addr={}:{}",
            nqn, spec.bdev_name, spec.listen_address, spec.listen_port
        );

        let tgt_ptr = self.get_tgt_ptr()?;

        let nqn_c = std::ffi::CString::new(nqn.as_str())
            .map_err(|e| FrontendError::Nvmf(format!("invalid NQN: {e}")))?;
        let bdev_c = std::ffi::CString::new(spec.bdev_name.as_str())
            .map_err(|e| FrontendError::Nvmf(format!("invalid bdev name: {e}")))?;
        let addr_c = std::ffi::CString::new(spec.listen_address.as_str())
            .map_err(|e| FrontendError::Nvmf(format!("invalid listen addr: {e}")))?;
        let port_str = spec.listen_port.to_string();
        let port_c = std::ffi::CString::new(port_str.as_str()).unwrap();

        // Phase 1: create subsystem + namespace + build trid.
        let (subsystem_send, trid_box) = reactor_dispatch::dispatch_sync(
            move || -> Result<(SendPtr, Box<ffi::spdk_nvme_transport_id>)> {
                unsafe {
                    let tgt = tgt_ptr.as_ptr() as *mut ffi::spdk_nvmf_tgt;
                    let subsystem = ffi::spdk_nvmf_subsystem_create(
                        tgt,
                        nqn_c.as_ptr() as *const c_char,
                        ffi::spdk_nvmf_subtype_SPDK_NVMF_SUBTYPE_NVME,
                        1,
                    );
                    if subsystem.is_null() {
                        return Err(FrontendError::Nvmf(format!(
                            "spdk_nvmf_subsystem_create failed for nqn={}",
                            nqn_c.to_str().unwrap_or("?")
                        )));
                    }
                    ffi::spdk_nvmf_subsystem_set_allow_any_host(subsystem, true);
                    ffi::spdk_nvmf_subsystem_allow_any_listener(subsystem, true);

                    let mut ns_opts: ffi::spdk_nvmf_ns_opts = std::mem::zeroed();
                    ffi::spdk_nvmf_ns_opts_get_defaults(
                        &mut ns_opts,
                        std::mem::size_of_val(&ns_opts),
                    );
                    let ns_id = ffi::spdk_nvmf_subsystem_add_ns_ext(
                        subsystem,
                        bdev_c.as_ptr() as *const c_char,
                        &ns_opts,
                        std::mem::size_of_val(&ns_opts),
                        std::ptr::null(),
                    );
                    if ns_id == 0 {
                        ffi::spdk_nvmf_subsystem_destroy(subsystem, None, std::ptr::null_mut());
                        return Err(FrontendError::Nvmf(format!(
                            "spdk_nvmf_subsystem_add_ns_ext failed for bdev={}",
                            bdev_c.to_str().unwrap_or("?")
                        )));
                    }

                    let mut trid: ffi::spdk_nvme_transport_id = std::mem::zeroed();
                    trid.trtype = ffi::spdk_nvme_transport_type_SPDK_NVME_TRANSPORT_TCP;
                    trid.adrfam = ffi::spdk_nvmf_adrfam_SPDK_NVMF_ADRFAM_IPV4;
                    let trstring = b"TCP\0";
                    std::ptr::copy_nonoverlapping(
                        trstring.as_ptr() as *const c_char,
                        trid.trstring.as_mut_ptr(),
                        trstring.len().min(trid.trstring.len()),
                    );
                    let addr_bytes = addr_c.as_bytes_with_nul();
                    let port_bytes = port_c.as_bytes_with_nul();
                    std::ptr::copy_nonoverlapping(
                        addr_bytes.as_ptr() as *const c_char,
                        trid.traddr.as_mut_ptr(),
                        addr_bytes.len().min(trid.traddr.len()),
                    );
                    std::ptr::copy_nonoverlapping(
                        port_bytes.as_ptr() as *const c_char,
                        trid.trsvcid.as_mut_ptr(),
                        port_bytes.len().min(trid.trsvcid.len()),
                    );
                    Ok((SendPtr::new(subsystem as *mut c_void), Box::new(trid)))
                }
            },
        )?;

        // Phase 2a: register the transport-level listener.
        {
            let trid_copy: ffi::spdk_nvme_transport_id = unsafe { std::ptr::read(&*trid_box) };
            let trid_clone = Box::new(trid_copy);
            let tgt_send_copy = tgt_ptr;
            let rc = reactor_dispatch::dispatch_sync(move || -> Result<i32> {
                unsafe {
                    let mut listen_opts: ffi::spdk_nvmf_listen_opts = std::mem::zeroed();
                    ffi::spdk_nvmf_listen_opts_init(
                        &mut listen_opts,
                        std::mem::size_of::<ffi::spdk_nvmf_listen_opts>(),
                    );
                    let tgt = tgt_send_copy.as_ptr() as *mut ffi::spdk_nvmf_tgt;
                    let rc = ffi::spdk_nvmf_tgt_listen_ext(
                        tgt,
                        &*trid_clone as *const ffi::spdk_nvme_transport_id,
                        &mut listen_opts,
                    );
                    Ok(rc)
                }
            })?;
            // -17 = EEXIST (listener already registered for this addr:port).
            if rc != 0 && rc != -17 {
                return Err(FrontendError::Nvmf(format!(
                    "spdk_nvmf_tgt_listen_ext failed: rc={rc}"
                )));
            }
        }

        // Phase 2b: add subsystem listener -> start subsystem.
        let completion = Arc::new(Completion::<i32>::new());
        let ctx = Box::new(TargetCreateCtx {
            subsystem: subsystem_send,
            trid: trid_box,
            completion: completion.clone(),
        });
        reactor_dispatch::send_to_reactor(move || {
            let ctx_ptr = Box::into_raw(ctx) as *mut c_void;
            unsafe {
                let tctx = &*(ctx_ptr as *const TargetCreateCtx);
                let subsystem = tctx.subsystem.as_ptr() as *mut ffi::spdk_nvmf_subsystem;
                let trid_ptr = &*tctx.trid as *const ffi::spdk_nvme_transport_id
                    as *mut ffi::spdk_nvme_transport_id;
                ffi::spdk_nvmf_subsystem_add_listener(
                    subsystem,
                    trid_ptr,
                    Some(listener_done_cb),
                    ctx_ptr,
                );
            }
        });
        let rc = completion.wait();
        if rc != 0 {
            return Err(FrontendError::Nvmf(format!(
                "create_target callback chain failed: rc={rc}"
            )));
        }
        Ok(())
    }

    fn delete_target(&self, volume_name: &str) -> Result<()> {
        let nqn = crate::nvmf::volume_nqn(volume_name);
        info!("frontend: deleting NVMe-oF subsystem nqn={}", nqn);

        let tgt_ptr = self.get_tgt_ptr()?;
        let nqn_owned = nqn.clone();
        let subsystem_ptr = reactor_dispatch::dispatch_sync(move || -> Result<Option<SendPtr>> {
            let nqn_c = std::ffi::CString::new(nqn_owned.as_str()).unwrap();
            unsafe {
                let tgt = tgt_ptr.as_ptr() as *mut ffi::spdk_nvmf_tgt;
                let subsystem =
                    ffi::spdk_nvmf_tgt_find_subsystem(tgt, nqn_c.as_ptr() as *const c_char);
                if subsystem.is_null() {
                    Ok(None)
                } else {
                    Ok(Some(SendPtr::new(subsystem as *mut c_void)))
                }
            }
        })?;

        let subsystem_send = match subsystem_ptr {
            Some(p) => p,
            None => return Ok(()),
        };

        let completion = Arc::new(Completion::<i32>::new());
        let ctx = Box::new(TargetDeleteCtx {
            subsystem: subsystem_send,
            completion: completion.clone(),
        });
        reactor_dispatch::send_to_reactor(move || {
            let ctx_ptr = Box::into_raw(ctx) as *mut c_void;
            unsafe { retry_subsystem_stop(ctx_ptr) };
        });
        let rc = completion.wait();
        if rc != 0 {
            return Err(FrontendError::Nvmf(format!(
                "delete_target callback chain failed: rc={rc}"
            )));
        }
        Ok(())
    }
}

#[async_trait]
impl NvmfTarget for SpdkNvmfTarget {
    async fn add_subsystem(&self, spec: &SubsystemSpec) -> Result<()> {
        // Override the spec's listen address/port with the target's
        // canonical pair so all subsystems share one TCP listener.
        let actual = SubsystemSpec {
            volume_name: spec.volume_name.clone(),
            size_bytes: spec.size_bytes,
            bdev_name: spec.bdev_name.clone(),
            listen_address: self.listen_address.clone(),
            listen_port: self.listen_port,
        };
        let spec_owned = actual.clone();
        let s_ptr = self as *const Self as usize;
        // Drive the SPDK calls on a blocking thread because they wait on
        // reactor completions via Condvars; tokio's dispatch_async lives
        // inside the reactor path itself.
        tokio::task::spawn_blocking(move || {
            // SAFETY: `self` outlives this future because the manager is
            // held in an Arc by the reconciler.
            let s = unsafe { &*(s_ptr as *const Self) };
            s.create_target(&spec_owned)
        })
        .await
        .map_err(|e| FrontendError::Nvmf(format!("join: {e}")))??;
        self.subsystems
            .lock()
            .unwrap()
            .insert(actual.volume_name.clone(), actual);
        Ok(())
    }

    async fn remove_subsystem(&self, volume_name: &str) -> Result<()> {
        // Drop the entry first so list_subsystems reflects the removal even
        // if SPDK takes a while to finalise.
        self.subsystems.lock().unwrap().remove(volume_name);
        let s_ptr = self as *const Self as usize;
        let name = volume_name.to_string();
        tokio::task::spawn_blocking(move || {
            let s = unsafe { &*(s_ptr as *const Self) };
            s.delete_target(&name)
        })
        .await
        .map_err(|e| FrontendError::Nvmf(format!("join: {e}")))??;
        Ok(())
    }

    async fn list_subsystems(&self) -> Result<Vec<String>> {
        Ok(self.subsystems.lock().unwrap().keys().cloned().collect())
    }
}

// ---------------------------------------------------------------------------
// SPDK callbacks — create chain
// ---------------------------------------------------------------------------

unsafe extern "C" fn listener_done_cb(cb_arg: *mut c_void, status: i32) {
    let ctx_ptr = cb_arg;
    if status != 0 {
        error!("spdk_nvmf_subsystem_add_listener failed: rc={}", status);
        let ctx = Box::from_raw(ctx_ptr as *mut TargetCreateCtx);
        ctx.completion.complete(status);
        return;
    }
    let ctx = &*(ctx_ptr as *const TargetCreateCtx);
    let subsystem = ctx.subsystem.as_ptr() as *mut ffi::spdk_nvmf_subsystem;
    let rc = ffi::spdk_nvmf_subsystem_start(subsystem, Some(create_start_done_cb), ctx_ptr);
    if rc != 0 {
        error!("spdk_nvmf_subsystem_start synchronous failure: rc={}", rc);
        let ctx = Box::from_raw(ctx_ptr as *mut TargetCreateCtx);
        ctx.completion.complete(rc);
    }
}

unsafe extern "C" fn create_start_done_cb(
    _subsystem: *mut ffi::spdk_nvmf_subsystem,
    cb_arg: *mut c_void,
    status: i32,
) {
    let ctx = Box::from_raw(cb_arg as *mut TargetCreateCtx);
    if status != 0 {
        error!("spdk_nvmf_subsystem_start failed: rc={}", status);
    }
    ctx.completion.complete(status);
}

// ---------------------------------------------------------------------------
// SPDK callbacks — delete chain (with EINPROGRESS retry)
// ---------------------------------------------------------------------------

unsafe fn retry_subsystem_stop(ctx_ptr: *mut c_void) {
    let dctx = &*(ctx_ptr as *const TargetDeleteCtx);
    let subsystem = dctx.subsystem.as_ptr() as *mut ffi::spdk_nvmf_subsystem;
    let rc = ffi::spdk_nvmf_subsystem_stop(subsystem, Some(delete_stop_done_cb), ctx_ptr);
    if rc == 0 {
        return;
    }
    if rc == EINPROGRESS {
        warn!(
            "spdk_nvmf_subsystem_stop EINPROGRESS, retrying in {}us",
            DELETE_RETRY_US
        );
        schedule_retry_on_reactor(ctx_ptr, retry_subsystem_stop);
        return;
    }
    error!("spdk_nvmf_subsystem_stop synchronous failure: rc={}", rc);
    let dctx = Box::from_raw(ctx_ptr as *mut TargetDeleteCtx);
    dctx.completion.complete(rc);
}

unsafe fn retry_subsystem_destroy(ctx_ptr: *mut c_void) {
    let dctx = &*(ctx_ptr as *const TargetDeleteCtx);
    let subsystem = dctx.subsystem.as_ptr() as *mut ffi::spdk_nvmf_subsystem;
    let rc = ffi::spdk_nvmf_subsystem_destroy(subsystem, Some(delete_destroy_done_cb), ctx_ptr);
    if rc == 0 {
        return;
    }
    if rc == EINPROGRESS {
        warn!(
            "spdk_nvmf_subsystem_destroy EINPROGRESS, retrying in {}us",
            DELETE_RETRY_US
        );
        schedule_retry_on_reactor(ctx_ptr, retry_subsystem_destroy);
        return;
    }
    error!("spdk_nvmf_subsystem_destroy synchronous failure: rc={}", rc);
    let dctx = Box::from_raw(ctx_ptr as *mut TargetDeleteCtx);
    dctx.completion.complete(rc);
}

fn schedule_retry_on_reactor(ctx_ptr: *mut c_void, retry_fn: unsafe fn(*mut c_void)) {
    let ptr = ctx_ptr as usize;
    std::thread::spawn(move || {
        std::thread::sleep(std::time::Duration::from_micros(DELETE_RETRY_US));
        reactor_dispatch::send_to_reactor(move || unsafe {
            retry_fn(ptr as *mut c_void);
        });
    });
}

unsafe extern "C" fn delete_stop_done_cb(
    _subsystem: *mut ffi::spdk_nvmf_subsystem,
    cb_arg: *mut c_void,
    status: i32,
) {
    if status != 0 {
        error!("spdk_nvmf_subsystem_stop callback failed: rc={}", status);
        let ctx = Box::from_raw(cb_arg as *mut TargetDeleteCtx);
        ctx.completion.complete(status);
        return;
    }
    retry_subsystem_destroy(cb_arg);
}

unsafe extern "C" fn delete_destroy_done_cb(cb_arg: *mut c_void) {
    let ctx = Box::from_raw(cb_arg as *mut TargetDeleteCtx);
    ctx.completion.complete(0);
}
