//! Background bitmap-destage hook.
//!
//! In architecture-v1 the volume bdev (`bdev::novanas_bdev`, now in
//! `storage/frontend`) maintained per-chunk dirty bitmaps that this module
//! drained to the metadata store on a 30s timer and on clean shutdown.
//!
//! In architecture-v2 the volume bdev lives in the frontend daemon, which
//! also owns the dirty-bitmap drain. The data daemon retains this module
//! solely so `main.rs` and any external callers can still call
//! `destage_all_bitmaps()` on shutdown without #ifdef'ing on the feature
//! flag — it is now a no-op.

/// No-op shutdown hook retained for source compatibility.
///
/// Returns immediately. Any caller that previously expected this to flush
/// volume-bdev dirty bitmaps now needs to call into the frontend daemon
/// instead.
pub fn destage_all_bitmaps() {
    log::debug!("destage_all_bitmaps: no-op in novanas-data (volume bdev moved to frontend)");
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn destage_is_a_noop() {
        // The contract is "doesn't panic, doesn't deadlock, returns".
        destage_all_bitmaps();
    }
}
