//! Storage backend interface.
//!
//! Single backend type: `RawDisk` (NVMe via vfio-pci, SATA via SPDK AIO).
//! `lvm` and `file` backends were deleted with the architecture-v2 strip;
//! see docs/16-data-meta-frontend.md for the rationale (single-host,
//! no-multibackend design).
//!
//! The ChunkEngine sits above the backend and stores content-addressed
//! 4MB chunks via the async `ChunkStore` trait.

#[cfg(feature = "spdk-sys")]
pub mod bdev_store;
pub mod chunk_store;
#[cfg(feature = "spdk-sys")]
pub mod raw_disk;
pub mod superblock;
pub mod traits;
