//! `TASK_KIND_RELEASE_DISK` handler.
//!
//! Tears down a previously-claimed disk:
//! - resolves the disk via [`disk_discovery`](crate::disk_discovery),
//! - zeroes the primary and tail superblock blocks (LBA 0 and the last
//!   8 sectors),
//! - rebinds the underlying NVMe device back to the kernel `nvme`
//!   driver when applicable.
//!
//! As with claim_disk, the filesystem touch points are abstracted behind
//! [`ReleaseDiskBackend`] for unit testing.

use std::path::{Path, PathBuf};

use crate::disk_discovery::{discover_in, DeviceClass, DeviceInfo};
use crate::error::{DataPlaneError, Result};
use crate::task_handlers::claim_disk::{SECTOR_SIZE, TAIL_RESERVED_SECTORS};
use crate::transport::meta_proto::ReleaseDiskTask;

use super::HandlerContext;
use crate::backend::superblock::SUPERBLOCK_SIZE;

pub trait ReleaseDiskBackend: Send + Sync {
    fn locate(&self, disk_uuid: &str) -> Option<DiskLocation>;
}

#[derive(Debug, Clone)]
pub struct DiskLocation {
    pub info: DeviceInfo,
    pub dev_path: PathBuf,
}

pub struct SysfsReleaseBackend {
    sysfs_root: PathBuf,
    dev_root: PathBuf,
}

impl SysfsReleaseBackend {
    pub fn new(sysfs_root: PathBuf, dev_root: PathBuf) -> Self {
        Self {
            sysfs_root,
            dev_root,
        }
    }
}

impl ReleaseDiskBackend for SysfsReleaseBackend {
    fn locate(&self, disk_uuid: &str) -> Option<DiskLocation> {
        let infos = discover_in(&self.sysfs_root.join("block")).ok()?;
        for info in infos {
            if info.wwn == disk_uuid || info.serial == disk_uuid {
                let dev_path = self.dev_root.join(&info.slot);
                return Some(DiskLocation { info, dev_path });
            }
        }
        None
    }
}

/// Result returned to the caller / runner.
#[derive(Debug, Clone)]
pub struct ReleaseOutcome {
    pub disk_uuid: String,
    pub slot: String,
    pub dev_path: PathBuf,
    pub class: DeviceClass,
}

/// Unit-testable core.
pub async fn handle_with_backend<B: ReleaseDiskBackend>(
    backend: &B,
    task: &ReleaseDiskTask,
) -> Result<ReleaseOutcome> {
    let location = backend.locate(&task.disk_uuid).ok_or_else(|| {
        DataPlaneError::BdevError(format!(
            "release_disk: disk {} not present on this host",
            task.disk_uuid
        ))
    })?;

    zero_superblock_pair(&location.dev_path, location.info.size_bytes)?;

    Ok(ReleaseOutcome {
        disk_uuid: task.disk_uuid.clone(),
        slot: location.info.slot,
        dev_path: location.dev_path,
        class: location.info.class,
    })
}

/// Production handler used by [`super::handle_task`].
pub async fn handle(ctx: &HandlerContext, task: &ReleaseDiskTask) -> Result<()> {
    let backend = SysfsReleaseBackend::new(ctx.sysfs_root.clone(), PathBuf::from("/dev"));
    let outcome = handle_with_backend(&backend, task).await?;

    // Best-effort: rebind NVMe back to the kernel driver.
    if outcome.class == DeviceClass::Nvme {
        if let Ok(bdf) = ctx.device_manager.pci_bdf_for_slot(&outcome.slot) {
            if let Err(e) = ctx.device_manager.rebind_kernel(&bdf) {
                log::warn!("release_disk: rebind {bdf} → kernel nvme failed: {e} (best-effort)");
            }
        }
    }
    log::info!(
        "release_disk: superblock zeroed on {}",
        outcome.dev_path.display()
    );
    Ok(())
}

fn zero_superblock_pair(dev_path: &Path, size_bytes: u64) -> Result<()> {
    use std::io::{Seek, SeekFrom, Write};

    if size_bytes < SUPERBLOCK_SIZE as u64 + TAIL_RESERVED_SECTORS * SECTOR_SIZE {
        return Err(DataPlaneError::BdevError(format!(
            "release_disk: device too small ({} bytes) for tail superblock",
            size_bytes
        )));
    }

    let zeros = [0u8; SUPERBLOCK_SIZE];
    let mut f = std::fs::OpenOptions::new()
        .write(true)
        .open(dev_path)
        .map_err(|e| {
            DataPlaneError::BdevError(format!(
                "release_disk: open {} for write: {e}",
                dev_path.display()
            ))
        })?;
    f.seek(SeekFrom::Start(0))?;
    f.write_all(&zeros)?;
    let tail_offset = size_bytes - TAIL_RESERVED_SECTORS * SECTOR_SIZE;
    f.seek(SeekFrom::Start(tail_offset))?;
    f.write_all(&zeros)?;
    f.sync_all()?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::backend::superblock::{DiskRole, Superblock, SUPERBLOCK_VERSION};
    use std::io::Write;

    struct FakeBackend {
        loc: DiskLocation,
    }
    impl ReleaseDiskBackend for FakeBackend {
        fn locate(&self, _disk_uuid: &str) -> Option<DiskLocation> {
            Some(self.loc.clone())
        }
    }

    fn make_disk_with_sb(dir: &Path, slot: &str, size_bytes: u64) -> PathBuf {
        let path = dir.join(slot);
        let mut f = std::fs::File::create(&path).unwrap();
        f.write_all(&vec![0u8; size_bytes as usize]).unwrap();
        // Write a valid superblock.
        let sb = Superblock {
            version: SUPERBLOCK_VERSION,
            pool_id: "pool-1".into(),
            role: DiskRole::Data,
            ..Superblock::default()
        };
        let buf = sb.marshal().unwrap();
        crate::backend::superblock::write_superblock(&path, &sb).unwrap();
        let _ = buf;
        // And to the tail.
        use std::io::{Seek, SeekFrom};
        let mut f = std::fs::OpenOptions::new().write(true).open(&path).unwrap();
        let tail_offset = size_bytes - TAIL_RESERVED_SECTORS * SECTOR_SIZE;
        let buf = sb.marshal().unwrap();
        f.seek(SeekFrom::Start(tail_offset)).unwrap();
        f.write_all(&buf).unwrap();
        f.sync_all().unwrap();
        path
    }

    fn fake(dir: &Path, slot: &str, size: u64) -> FakeBackend {
        let dev_path = make_disk_with_sb(dir, slot, size);
        FakeBackend {
            loc: DiskLocation {
                info: DeviceInfo {
                    slot: slot.into(),
                    wwn: "test-wwn".into(),
                    vendor: "T".into(),
                    model: "T".into(),
                    serial: "T".into(),
                    size_bytes: size,
                    class: DeviceClass::Hdd,
                },
                dev_path,
            },
        }
    }

    #[tokio::test]
    async fn zeros_primary_and_tail_superblocks() {
        let dir = tempfile::tempdir().unwrap();
        let size = 64 * 1024;
        let b = fake(dir.path(), "rd0", size);

        // Sanity: starts with valid magic.
        let pre = std::fs::read(&b.loc.dev_path).unwrap();
        assert_eq!(&pre[..8], b"NOVANAS\0");

        let task = ReleaseDiskTask {
            disk_uuid: "test-wwn".into(),
        };
        let outcome = handle_with_backend(&b, &task).await.unwrap();
        assert_eq!(outcome.slot, "rd0");

        let post = std::fs::read(&outcome.dev_path).unwrap();
        // Primary zeroed.
        assert!(post[..SUPERBLOCK_SIZE].iter().all(|&b| b == 0));
        // Tail zeroed.
        let tail_off = (size - TAIL_RESERVED_SECTORS * SECTOR_SIZE) as usize;
        assert!(post[tail_off..tail_off + SUPERBLOCK_SIZE]
            .iter()
            .all(|&b| b == 0));
    }

    #[tokio::test]
    async fn missing_disk_returns_error() {
        struct EmptyBackend;
        impl ReleaseDiskBackend for EmptyBackend {
            fn locate(&self, _: &str) -> Option<DiskLocation> {
                None
            }
        }
        let err = handle_with_backend(
            &EmptyBackend,
            &ReleaseDiskTask {
                disk_uuid: "nope".into(),
            },
        )
        .await
        .unwrap_err();
        assert!(format!("{err}").contains("not present"));
    }
}
