//! SPDK bdev modules retained on the data daemon.
//!
//! With architecture-v2, the volume bdev (`novanas_bdev`), client-side
//! ChunkEngine plumbing (`chunk_io`), and cross-disk EC/replica fan-out
//! (`replica`, `novanas_replica_bdev`, `erasure`) have moved out of this
//! crate:
//! - the volume bdev + chunk client live in `storage/frontend`,
//! - cross-disk EC repair is being rebuilt on top of `policy::operations`
//!   driven by `ChunkOpTask` and is not yet present in this crate.
//!
//! Only the sub-block helper survives — the chunk store still needs the
//! 64 KiB sub-block partitioning math.

pub mod sub_block;
