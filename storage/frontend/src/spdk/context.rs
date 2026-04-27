//! SPDK reactor completion-channel utilities (placeholder port).
//!
//! See the note in `env.rs` — the full implementation lives in
//! `dataplane/src/spdk/context.rs` and will land in the frontend
//! verbatim once Agent C completes the split.

use std::sync::{Condvar, Mutex};

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
