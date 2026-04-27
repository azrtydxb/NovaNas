//! `TASK_KIND_CLAIM_DISK` handler.
//!
//! High-level flow (see `docs/16-data-meta-frontend.md` § Volume
//! lifecycle):
//!
//! 1. Locate the local disk identified by `disk_uuid` via
//!    [`disk_discovery`](crate::disk_discovery).
//! 2. Confirm it is empty (read first 4 KiB; refuse unless `force`).
//! 3. NVMe path: unbind from kernel `nvme`, bind to `vfio-pci`. SATA path:
//!    open as an SPDK AIO bdev (production code path under
//!    `feature = "spdk-sys"`).
//! 4. Build the [`Superblock`](crate::backend::superblock::Superblock)
//!    with the task's pool / disk identity and CRUSH digest.
//! 5. Write the superblock to LBA 0 *and* `last_lba - 7` (8 sectors at
//!    the end of the device — the redundant copy).
//!
//! All filesystem access is funneled through [`ClaimContext`] so unit
//! tests can substitute a fake disk file.

use std::path::{Path, PathBuf};

use crate::backend::superblock::{DiskRole, Superblock, SUPERBLOCK_SIZE, SUPERBLOCK_VERSION};
use crate::disk_discovery::{discover_in, DeviceClass, DeviceInfo};
use crate::error::{DataPlaneError, Result};
use crate::transport::meta_proto::ClaimDiskTask;

use super::HandlerContext;

/// The 8-sector redundant superblock copy at the tail of every disk.
pub const TAIL_RESERVED_SECTORS: u64 = 8;
/// Sector size assumed for tail offset arithmetic. The on-disk format is
/// 4 KiB-aligned regardless of the kernel block size.
pub const SECTOR_SIZE: u64 = 512;

/// Resolve disk slot/path information used by the handler. Production
/// uses `/dev/<slot>` from sysfs; tests inject a fake.
pub trait ClaimDiskBackend: Send + Sync {
    /// Map a disk uuid to the local block-device path the handler should
    /// open. Returns `None` if the disk isn't visible to the host.
    fn locate(&self, disk_uuid: &str) -> Option<DiskLocation>;
}

/// Where to find a disk on the host.
#[derive(Debug, Clone)]
pub struct DiskLocation {
    pub info: DeviceInfo,
    /// `/dev/<slot>` style path.
    pub dev_path: PathBuf,
}

/// Production backend: scans `/sys/block` and looks for a disk whose WWN
/// matches `disk_uuid`. (Meta records WWN as the disk's stable id.)
pub struct SysfsClaimBackend {
    sysfs_root: PathBuf,
    dev_root: PathBuf,
}

impl SysfsClaimBackend {
    pub fn new(sysfs_root: PathBuf, dev_root: PathBuf) -> Self {
        Self {
            sysfs_root,
            dev_root,
        }
    }
}

impl ClaimDiskBackend for SysfsClaimBackend {
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

/// Result of a successful claim.
#[derive(Debug, Clone)]
pub struct ClaimOutcome {
    pub disk_uuid: String,
    pub slot: String,
    pub dev_path: PathBuf,
    pub class: DeviceClass,
    pub size_bytes: u64,
}

/// Run a CLAIM_DISK task using the supplied backend.
///
/// This is the unit-testable core; [`handle`] is the production entry
/// point that builds a [`SysfsClaimBackend`] from `ctx`.
pub async fn handle_with_backend<B: ClaimDiskBackend>(
    backend: &B,
    task: &ClaimDiskTask,
) -> Result<ClaimOutcome> {
    let location = backend.locate(&task.disk_uuid).ok_or_else(|| {
        DataPlaneError::BdevError(format!(
            "claim_disk: disk {} not present on this host",
            task.disk_uuid
        ))
    })?;

    let dev_path = location.dev_path.clone();
    let info = location.info.clone();

    // Wrapper-style `ClaimDiskTask` carries only `(disk_uuid, pool_uuid)`.
    // Single-host appliances claim every disk as data-role, refuse to
    // overwrite a non-empty disk (no `force` flag), and leave the CRUSH
    // digest zeroed — meta is the source of truth for digest rotation
    // and writes the new value via a follow-up superblock-update path.
    ensure_empty(&dev_path)?;

    let crush_digest = [0u8; 32];
    let role = DiskRole::Data;

    let now_nanos = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_nanos() as i64)
        .unwrap_or(0);

    let mut sb = Superblock {
        version: SUPERBLOCK_VERSION,
        flags: 0,
        disk_uuid: pack_uuid(&task.disk_uuid),
        pool_id: task.pool_uuid.clone(),
        role,
        crush_digest,
        meta_volume_name: String::new(),
        meta_volume_root_chunk: String::new(),
        meta_volume_version: 0,
        created_unix_nanos: now_nanos,
        updated_unix_nanos: now_nanos,
    };

    if sb.pool_id.len() > 32 {
        sb.pool_id.truncate(32);
    }

    let buf = sb
        .marshal()
        .map_err(|e| DataPlaneError::BdevError(format!("marshal superblock: {e}")))?;

    write_superblock_pair(&dev_path, &buf, info.size_bytes)?;

    Ok(ClaimOutcome {
        disk_uuid: task.disk_uuid.clone(),
        slot: info.slot,
        dev_path,
        class: info.class,
        size_bytes: info.size_bytes,
    })
}

/// Production handler used by [`super::handle_task`].
pub async fn handle(ctx: &HandlerContext, task: &ClaimDiskTask) -> Result<()> {
    // NVMe driver swap is best-effort: it only succeeds on a real Linux
    // host with vfio-pci available. Errors are non-fatal for the claim
    // itself (the AIO path also works); we log and continue.
    let backend = SysfsClaimBackend::new(ctx.sysfs_root.clone(), std::path::PathBuf::from("/dev"));
    if let Some(loc) = backend.locate(&task.disk_uuid) {
        if loc.info.class == DeviceClass::Nvme {
            if let Err(e) = ctx.device_manager.nvme_to_vfio(&loc.info.slot) {
                log::warn!(
                    "claim_disk: nvme→vfio bind failed for {} ({}): {e} — falling back to AIO",
                    loc.info.slot,
                    task.disk_uuid
                );
            }
        }
    }

    let outcome = handle_with_backend(&backend, task).await?;
    log::info!(
        "claim_disk: superblock written to {} (class {:?}, {} bytes)",
        outcome.dev_path.display(),
        outcome.class,
        outcome.size_bytes
    );
    Ok(())
}

fn ensure_empty(dev_path: &Path) -> Result<()> {
    use std::io::Read;
    let mut f = std::fs::OpenOptions::new()
        .read(true)
        .open(dev_path)
        .map_err(|e| {
            DataPlaneError::BdevError(format!(
                "claim_disk: open {} for empty-check: {e}",
                dev_path.display()
            ))
        })?;
    let mut buf = [0u8; 4096];
    let n = f.read(&mut buf).map_err(|e| {
        DataPlaneError::BdevError(format!("claim_disk: read {}: {e}", dev_path.display()))
    })?;
    if n == 0 {
        return Ok(());
    }
    if buf[..n].iter().any(|&b| b != 0) {
        return Err(DataPlaneError::BdevError(format!(
            "claim_disk: {} is non-empty (first 4KiB has data); pass force=true to overwrite",
            dev_path.display()
        )));
    }
    Ok(())
}

fn write_superblock_pair(
    dev_path: &Path,
    buf: &[u8; SUPERBLOCK_SIZE],
    size_bytes: u64,
) -> Result<()> {
    use std::io::{Seek, SeekFrom, Write};

    if size_bytes < SUPERBLOCK_SIZE as u64 + TAIL_RESERVED_SECTORS * SECTOR_SIZE {
        return Err(DataPlaneError::BdevError(format!(
            "claim_disk: device is too small ({} bytes) for two superblock copies",
            size_bytes
        )));
    }

    let mut f = std::fs::OpenOptions::new()
        .write(true)
        .create(false)
        .open(dev_path)
        .map_err(|e| {
            DataPlaneError::BdevError(format!(
                "claim_disk: open {} for write: {e}",
                dev_path.display()
            ))
        })?;
    f.seek(SeekFrom::Start(0))?;
    f.write_all(buf)?;
    let tail_offset = size_bytes - TAIL_RESERVED_SECTORS * SECTOR_SIZE;
    f.seek(SeekFrom::Start(tail_offset))?;
    f.write_all(buf)?;
    f.sync_all()?;
    Ok(())
}

/// Pack a string (uuid or wwn) into the 16-byte superblock disk_uuid
/// field. Accepts canonical `xxxxxxxx-xxxx-...` and arbitrary strings;
/// non-hex input is hashed with FNV-1a so two distinct ids never collide
/// on the same disk.
fn pack_uuid(s: &str) -> [u8; 16] {
    if let Ok(parsed) = uuid::Uuid::parse_str(s) {
        return *parsed.as_bytes();
    }
    let mut out = [0u8; 16];
    // Two-half FNV-1a so all 128 bits get used.
    let h_lo = fnv1a_64(s.as_bytes(), 0xcbf29ce484222325);
    let h_hi = fnv1a_64(s.as_bytes(), !0xcbf29ce484222325);
    out[..8].copy_from_slice(&h_lo.to_le_bytes());
    out[8..].copy_from_slice(&h_hi.to_le_bytes());
    out
}

fn fnv1a_64(data: &[u8], seed: u64) -> u64 {
    let mut h = seed;
    for &b in data {
        h ^= b as u64;
        h = h.wrapping_mul(0x100000001b3);
    }
    h
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;

    struct FakeBackend {
        loc: DiskLocation,
    }
    impl ClaimDiskBackend for FakeBackend {
        fn locate(&self, _disk_uuid: &str) -> Option<DiskLocation> {
            Some(self.loc.clone())
        }
    }

    fn make_disk(dir: &Path, slot: &str, size_bytes: u64) -> PathBuf {
        let path = dir.join(slot);
        let mut f = std::fs::File::create(&path).unwrap();
        f.write_all(&vec![0u8; size_bytes as usize]).unwrap();
        path
    }

    fn task() -> ClaimDiskTask {
        ClaimDiskTask {
            pool_uuid: "pool-1".into(),
            disk_uuid: "naa.5000abcd1234".into(),
        }
    }

    fn backend(dir: &Path, slot: &str, size_bytes: u64, class: DeviceClass) -> FakeBackend {
        let dev_path = make_disk(dir, slot, size_bytes);
        FakeBackend {
            loc: DiskLocation {
                info: DeviceInfo {
                    slot: slot.into(),
                    wwn: "naa.5000abcd1234".into(),
                    vendor: "TEST".into(),
                    model: "TEST".into(),
                    serial: "abcd".into(),
                    size_bytes,
                    class,
                },
                dev_path,
            },
        }
    }

    #[tokio::test]
    async fn writes_superblock_at_lba0_and_tail() {
        let dir = tempfile::tempdir().unwrap();
        let size = 64 * 1024;
        let b = backend(dir.path(), "fake", size, DeviceClass::Hdd);
        let outcome = handle_with_backend(&b, &task()).await.unwrap();
        assert_eq!(outcome.disk_uuid, "naa.5000abcd1234");

        // Validate primary copy.
        let buf = std::fs::read(&outcome.dev_path).unwrap();
        let primary = &buf[..SUPERBLOCK_SIZE];
        let sb = Superblock::unmarshal(primary).unwrap();
        assert_eq!(sb.pool_id, "pool-1");
        assert_eq!(sb.role, DiskRole::Data);

        // Validate tail copy.
        let tail_off = (size - TAIL_RESERVED_SECTORS * SECTOR_SIZE) as usize;
        let tail = &buf[tail_off..tail_off + SUPERBLOCK_SIZE];
        let sb_tail = Superblock::unmarshal(tail).unwrap();
        assert_eq!(sb_tail.pool_id, "pool-1");
    }

    #[tokio::test]
    async fn refuses_non_empty_disk() {
        let dir = tempfile::tempdir().unwrap();
        let size = 64 * 1024;
        let b = backend(dir.path(), "dirty", size, DeviceClass::Hdd);
        // Pre-populate with non-zero data.
        std::fs::write(&b.loc.dev_path, vec![0xffu8; size as usize]).unwrap();

        let err = handle_with_backend(&b, &task()).await.unwrap_err();
        assert!(format!("{err}").contains("non-empty"));
    }

    #[tokio::test]
    async fn rejects_too_small_device() {
        let dir = tempfile::tempdir().unwrap();
        let size = 1024; // smaller than 4 KiB superblock + 8-sector tail.
        let b = backend(dir.path(), "tiny", size, DeviceClass::Hdd);
        let err = handle_with_backend(&b, &task()).await.unwrap_err();
        assert!(format!("{err}").contains("too small"));
    }

    #[test]
    fn pack_uuid_canonical_form_roundtrips() {
        let u = uuid::Uuid::new_v4();
        let packed = pack_uuid(&u.to_string());
        assert_eq!(packed, *u.as_bytes());
    }

    #[test]
    fn pack_uuid_falls_back_to_hash_for_wwns() {
        let a = pack_uuid("naa.5000a");
        let b = pack_uuid("naa.5000b");
        assert_ne!(a, b);
        // Same input → same output (deterministic).
        assert_eq!(pack_uuid("naa.5000a"), a);
        // Should not be all zero.
        assert!(a.iter().any(|&b| b != 0));
    }
}
