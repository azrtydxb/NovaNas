//! SPDK thread dispatch utilities.
//!
//! Ported verbatim from `storage/dataplane/src/spdk/context.rs`. SPDK
//! operations must run on the SPDK app thread (reactor). The frontend's
//! tonic / tokio threads dispatch work to the reactor via
//! `spdk_thread_send_msg` and either block on a [`Completion`] or `.await`
//! an [`AsyncCompletion`].

use std::sync::{Condvar, Mutex};

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

/// A one-shot completion channel for synchronising an SPDK callback result
/// back to a waiting caller thread.
pub struct Completion<T> {
    inner: Mutex<Option<T>>,
    cond: Condvar,
}

impl<T> Default for Completion<T> {
    fn default() -> Self {
        Self::new()
    }
}

impl<T> Completion<T> {
    pub fn new() -> Self {
        Self {
            inner: Mutex::new(None),
            cond: Condvar::new(),
        }
    }

    pub fn wait(&self) -> T {
        let mut guard = self.inner.lock().unwrap();
        while guard.is_none() {
            guard = self.cond.wait(guard).unwrap();
        }
        guard.take().unwrap()
    }

    pub fn complete(&self, value: T) {
        let mut guard = self.inner.lock().unwrap();
        *guard = Some(value);
        self.cond.notify_one();
    }

    pub fn as_ptr(&self) -> *mut std::os::raw::c_void {
        self as *const Self as *mut std::os::raw::c_void
    }

    /// # Safety
    /// The pointer must originate from `as_ptr` on a live `Completion`.
    pub unsafe fn from_ptr<'a>(ptr: *mut std::os::raw::c_void) -> &'a Self {
        &*(ptr as *const Self)
    }
}

/// Dispatch a closure to the SPDK reactor (app) thread and block until it
/// completes, returning the result.
pub fn dispatch_to_reactor<F, R>(f: F) -> R
where
    F: FnOnce() -> R + Send + 'static,
    R: Send + 'static,
{
    // If we're already on an SPDK thread, execute directly.
    let on_spdk_thread = unsafe { !ffi::spdk_get_thread().is_null() };
    if on_spdk_thread {
        return f();
    }

    let completion = std::sync::Arc::new(Completion::<R>::new());
    let completion_clone = completion.clone();

    struct DispatchCtx<F, R> {
        func: Option<F>,
        completion: std::sync::Arc<Completion<R>>,
    }

    let ctx = Box::new(DispatchCtx {
        func: Some(f),
        completion: completion_clone,
    });
    let ctx_ptr = Box::into_raw(ctx) as *mut std::os::raw::c_void;

    unsafe extern "C" fn dispatch_cb<F, R>(arg: *mut std::os::raw::c_void)
    where
        F: FnOnce() -> R + Send + 'static,
        R: Send + 'static,
    {
        let mut ctx = Box::from_raw(arg as *mut DispatchCtx<F, R>);
        let func = ctx.func.take().unwrap();
        let result = func();
        ctx.completion.complete(result);
    }

    unsafe {
        let app_thread = ffi::spdk_thread_get_app_thread();
        assert!(!app_thread.is_null(), "SPDK app thread not available");

        let rc = ffi::spdk_thread_send_msg(app_thread, Some(dispatch_cb::<F, R>), ctx_ptr);
        assert!(rc == 0, "spdk_thread_send_msg failed: rc={}", rc);
    }

    completion.wait()
}

/// An async-compatible one-shot completion channel using tokio::sync::oneshot.
pub struct AsyncCompletion<T> {
    rx: tokio::sync::oneshot::Receiver<T>,
}

/// The sender half of an async completion, passed to SPDK callbacks.
pub struct AsyncCompletionSender<T> {
    tx: Option<tokio::sync::oneshot::Sender<T>>,
}

impl<T> AsyncCompletion<T> {
    pub fn new() -> (Self, AsyncCompletionSender<T>) {
        let (tx, rx) = tokio::sync::oneshot::channel();
        (
            AsyncCompletion { rx },
            AsyncCompletionSender { tx: Some(tx) },
        )
    }

    pub async fn wait(self) -> T {
        self.rx
            .await
            .expect("AsyncCompletion sender dropped without sending")
    }
}

impl<T> AsyncCompletionSender<T> {
    pub fn complete(&mut self, value: T) {
        if let Some(tx) = self.tx.take() {
            let _ = tx.send(value);
        }
    }

    pub fn into_ptr(self) -> *mut std::os::raw::c_void {
        Box::into_raw(Box::new(self)) as *mut std::os::raw::c_void
    }

    /// # Safety
    /// The pointer must have been created by [`into_ptr`] and not yet consumed.
    pub unsafe fn from_ptr(ptr: *mut std::os::raw::c_void) -> Self {
        *Box::from_raw(ptr as *mut Self)
    }
}
