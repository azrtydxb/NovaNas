//! NVMe driver-binding helper.
//!
//! Manages the `nvme` ↔ `vfio-pci` driver swap used by `claim_disk` for
//! NVMe SSDs. SATA paths are handled by SPDK's AIO bdev — they don't go
//! through here.
//!
//! The implementation reflects the standard sysfs dance:
//!
//! 1. Resolve `/sys/block/<slot>/device` → PCI BDF (e.g. `0000:01:00.0`).
//! 2. Read the current driver via `/sys/bus/pci/devices/<bdf>/driver`.
//! 3. If non-empty, write `<bdf>` to that driver's `unbind`.
//! 4. Append the device's vendor:device id to
//!    `/sys/bus/pci/drivers/vfio-pci/new_id` (idempotent — a duplicate
//!    write returns `EEXIST` which is treated as success).
//! 5. Write `<bdf>` to `/sys/bus/pci/drivers/vfio-pci/bind`.
//!
//! All filesystem writes are routed through the [`Manager`] type so unit
//! tests can target a fake sysfs root inside `tempdir()`.

use std::path::{Path, PathBuf};

use crate::error::{DataPlaneError, Result};

/// Default real sysfs root (`/sys`).
pub const DEFAULT_SYSFS_ROOT: &str = "/sys";

/// Manages the nvme ↔ vfio-pci driver binding for a single host.
#[derive(Debug, Clone)]
pub struct Manager {
    sysfs_root: PathBuf,
}

impl Default for Manager {
    fn default() -> Self {
        Self::with_sysfs_root(DEFAULT_SYSFS_ROOT)
    }
}

impl Manager {
    /// Construct a manager rooted at `sysfs_root` (production: `/sys`).
    pub fn with_sysfs_root(sysfs_root: impl Into<PathBuf>) -> Self {
        Self {
            sysfs_root: sysfs_root.into(),
        }
    }

    /// Resolve `/sys/block/<slot>/device` to a PCI BDF address.
    ///
    /// For NVMe namespaces (`nvmeXnY`) the device link points to the
    /// controller (`/sys/class/nvme/nvmeX`); we then walk that directory's
    /// own `device` symlink to reach the PCI BDF.
    pub fn pci_bdf_for_slot(&self, slot: &str) -> Result<String> {
        let device_link = self.sysfs_root.join("block").join(slot).join("device");
        let resolved = std::fs::canonicalize(&device_link).map_err(|e| {
            DataPlaneError::BdevError(format!("resolve {} → device: {e}", device_link.display()))
        })?;

        // For NVMe controllers the symlink lands inside
        // /sys/devices/pci.../0000:XX:YY.Z/nvme/nvme0; walk parents until
        // we find a directory whose name parses as a BDF.
        let mut cur: Option<&Path> = Some(resolved.as_path());
        while let Some(p) = cur {
            if let Some(name) = p.file_name().and_then(|s| s.to_str()) {
                if is_pci_bdf(name) {
                    return Ok(name.to_string());
                }
            }
            cur = p.parent();
        }
        Err(DataPlaneError::BdevError(format!(
            "no PCI BDF ancestor for {}",
            resolved.display()
        )))
    }

    /// Unbind `bdf` from its current kernel driver, if any.
    pub fn unbind_current(&self, bdf: &str) -> Result<()> {
        let driver_link = self
            .sysfs_root
            .join("bus/pci/devices")
            .join(bdf)
            .join("driver");
        match std::fs::canonicalize(&driver_link) {
            Ok(driver_dir) => {
                let unbind = driver_dir.join("unbind");
                std::fs::write(&unbind, bdf.as_bytes()).map_err(|e| {
                    DataPlaneError::BdevError(format!("unbind {bdf} via {}: {e}", unbind.display()))
                })?;
                Ok(())
            }
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
            Err(e) => Err(DataPlaneError::BdevError(format!(
                "lookup driver for {bdf}: {e}"
            ))),
        }
    }

    /// Bind `bdf` to `vfio-pci`, registering its vendor:device id first.
    pub fn bind_vfio_pci(&self, bdf: &str) -> Result<()> {
        let dev_dir = self.sysfs_root.join("bus/pci/devices").join(bdf);
        let vendor = read_id(&dev_dir.join("vendor"))
            .map_err(|e| DataPlaneError::BdevError(format!("read vendor for {bdf}: {e}")))?;
        let device = read_id(&dev_dir.join("device"))
            .map_err(|e| DataPlaneError::BdevError(format!("read device id for {bdf}: {e}")))?;

        let drv = self.sysfs_root.join("bus/pci/drivers/vfio-pci");
        let new_id_line = format!("{vendor:04x} {device:04x}");
        // EEXIST when this id is already registered with vfio-pci is fine.
        if let Err(e) = std::fs::write(drv.join("new_id"), new_id_line.as_bytes()) {
            if e.raw_os_error() != Some(libc::EEXIST)
                && e.kind() != std::io::ErrorKind::AlreadyExists
            {
                return Err(DataPlaneError::BdevError(format!(
                    "write vfio-pci/new_id: {e}"
                )));
            }
        }
        std::fs::write(drv.join("bind"), bdf.as_bytes())
            .map_err(|e| DataPlaneError::BdevError(format!("vfio-pci bind {bdf}: {e}")))?;
        Ok(())
    }

    /// Reverse of [`bind_vfio_pci`]: unbind from vfio-pci and re-bind to
    /// the kernel `nvme` driver.
    pub fn rebind_kernel(&self, bdf: &str) -> Result<()> {
        let drv = self.sysfs_root.join("bus/pci/drivers/vfio-pci");
        // Best-effort unbind from vfio-pci.
        let _ = std::fs::write(drv.join("unbind"), bdf.as_bytes());
        let nvme = self.sysfs_root.join("bus/pci/drivers/nvme");
        std::fs::write(nvme.join("bind"), bdf.as_bytes())
            .map_err(|e| DataPlaneError::BdevError(format!("nvme bind {bdf}: {e}")))
    }

    /// One-shot helper used by the CLAIM_DISK task handler: resolve slot,
    /// unbind from kernel `nvme`, bind to `vfio-pci`. Returns the BDF.
    pub fn nvme_to_vfio(&self, slot: &str) -> Result<String> {
        let bdf = self.pci_bdf_for_slot(slot)?;
        self.unbind_current(&bdf)?;
        self.bind_vfio_pci(&bdf)?;
        Ok(bdf)
    }
}

fn is_pci_bdf(name: &str) -> bool {
    // Format: domain:bus:device.function — `0000:01:00.0`
    let parts: Vec<_> = name.split([':', '.']).collect();
    if parts.len() != 4 {
        return false;
    }
    let lens = [4, 2, 2, 1];
    for (i, p) in parts.iter().enumerate() {
        if p.len() != lens[i] || !p.chars().all(|c| c.is_ascii_hexdigit()) {
            return false;
        }
    }
    true
}

fn read_id(path: &Path) -> std::io::Result<u32> {
    let raw = std::fs::read_to_string(path)?;
    let trimmed = raw.trim();
    let stripped = trimmed.strip_prefix("0x").unwrap_or(trimmed);
    u32::from_str_radix(stripped, 16)
        .map_err(|e| std::io::Error::new(std::io::ErrorKind::InvalidData, e))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn is_pci_bdf_accepts_canonical_form() {
        assert!(is_pci_bdf("0000:01:00.0"));
        assert!(is_pci_bdf("0001:ff:1f.7"));
    }

    #[test]
    fn is_pci_bdf_rejects_garbage() {
        assert!(!is_pci_bdf("01:00.0"));
        assert!(!is_pci_bdf("0000:01:00"));
        assert!(!is_pci_bdf("XXXX:01:00.0"));
        assert!(!is_pci_bdf("nvme0"));
    }

    fn fake_sysfs(root: &Path, slot: &str, bdf: &str, vendor: u32, device: u32) {
        // Real device dir under /sys/devices/pci.../0000:01:00.0/...
        let dev_dir = root.join("devices/pci0000:00").join(bdf);
        std::fs::create_dir_all(&dev_dir).unwrap();
        std::fs::write(dev_dir.join("vendor"), format!("0x{vendor:04x}\n")).unwrap();
        std::fs::write(dev_dir.join("device"), format!("0x{device:04x}\n")).unwrap();

        // /sys/bus/pci/devices/<bdf> → symlink target.
        let bus_devs = root.join("bus/pci/devices");
        std::fs::create_dir_all(&bus_devs).unwrap();
        std::os::unix::fs::symlink(&dev_dir, bus_devs.join(bdf)).unwrap();

        // /sys/bus/pci/drivers/vfio-pci/{bind,new_id}.
        let vfio = root.join("bus/pci/drivers/vfio-pci");
        std::fs::create_dir_all(&vfio).unwrap();
        std::fs::write(vfio.join("bind"), b"").unwrap();
        std::fs::write(vfio.join("new_id"), b"").unwrap();
        std::fs::write(vfio.join("unbind"), b"").unwrap();
        let nvme = root.join("bus/pci/drivers/nvme");
        std::fs::create_dir_all(&nvme).unwrap();
        std::fs::write(nvme.join("bind"), b"").unwrap();
        std::fs::write(nvme.join("unbind"), b"").unwrap();

        // /sys/block/<slot>/device → symlink to dev_dir/nvme/<slot>.
        let nvme_inner = dev_dir.join("nvme").join(slot.split('n').next().unwrap());
        std::fs::create_dir_all(&nvme_inner).unwrap();
        let block_dir = root.join("block").join(slot);
        std::fs::create_dir_all(&block_dir).unwrap();
        std::os::unix::fs::symlink(&nvme_inner, block_dir.join("device")).unwrap();
    }

    #[test]
    fn pci_bdf_for_slot_resolves_nvme() {
        let dir = tempfile::tempdir().unwrap();
        fake_sysfs(dir.path(), "nvme0n1", "0000:01:00.0", 0x144d, 0xa80a);
        let mgr = Manager::with_sysfs_root(dir.path());
        let bdf = mgr.pci_bdf_for_slot("nvme0n1").unwrap();
        assert_eq!(bdf, "0000:01:00.0");
    }

    #[test]
    fn unbind_with_no_driver_is_ok() {
        let dir = tempfile::tempdir().unwrap();
        fake_sysfs(dir.path(), "nvme0n1", "0000:01:00.0", 0x144d, 0xa80a);
        // No driver/ symlink under devices/<bdf>/ → unbind is a no-op.
        let mgr = Manager::with_sysfs_root(dir.path());
        mgr.unbind_current("0000:01:00.0").unwrap();
    }

    #[test]
    fn bind_vfio_pci_writes_new_id_and_bind() {
        let dir = tempfile::tempdir().unwrap();
        fake_sysfs(dir.path(), "nvme0n1", "0000:01:00.0", 0x144d, 0xa80a);
        let mgr = Manager::with_sysfs_root(dir.path());
        mgr.bind_vfio_pci("0000:01:00.0").unwrap();
        let new_id =
            std::fs::read_to_string(dir.path().join("bus/pci/drivers/vfio-pci/new_id")).unwrap();
        assert!(new_id.contains("144d"));
        assert!(new_id.contains("a80a"));
        let bind =
            std::fs::read_to_string(dir.path().join("bus/pci/drivers/vfio-pci/bind")).unwrap();
        assert!(bind.contains("0000:01:00.0"));
    }
}
