//! Linux block-device discovery for the data daemon.
//!
//! Walks `/sys/block`, building a `DeviceInfo` for every disk-like child
//! and inferring its [`DeviceClass`] from the rotational + queue/discard
//! hints in sysfs. Mirrors the Go agent's
//! `storage/internal/disk/discovery.go` behaviour and is the input the
//! data daemon ships up to `novanas-meta` in heartbeats.
//!
//! `sysfs_root` is configurable so the unit tests can build a fake
//! sysfs tree under `tempfile::tempdir()`.

use std::path::Path;

use crate::error::{DataPlaneError, Result};
use crate::transport::meta_proto::Disk;

/// Default sysfs root.
pub const DEFAULT_SYSFS_ROOT: &str = "/sys/block";

/// Internal classification of a discovered disk. The wrapper-style meta
/// proto does not carry a device-class field; the dataplane keeps the
/// distinction in-memory because handlers (NVMe driver swap, write cache
/// sizing, etc.) need it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DeviceClass {
    Unknown,
    Hdd,
    Ssd,
    Nvme,
}

impl DeviceClass {
    /// Stable string label used in logs and as the `tier` hint pushed to
    /// meta on `PutDisk`.
    pub fn as_str(self) -> &'static str {
        match self {
            DeviceClass::Unknown => "",
            DeviceClass::Hdd => "hdd",
            DeviceClass::Ssd => "ssd",
            DeviceClass::Nvme => "nvme",
        }
    }
}

/// Best-effort hardware/identification fields for a discovered disk.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DeviceInfo {
    /// Kernel slot, e.g. `sda` / `nvme0n1`.
    pub slot: String,
    /// World-wide name (`naa.…`) when available, otherwise `<slot>:<serial>`.
    pub wwn: String,
    /// Vendor string when published in sysfs (`device/vendor`).
    pub vendor: String,
    /// Model string (`device/model`).
    pub model: String,
    /// Serial number (`device/serial` or `device/wwid`).
    pub serial: String,
    /// Total size in bytes (`size` * 512, the kernel's sector multiplier).
    pub size_bytes: u64,
    /// Inferred device class.
    pub class: DeviceClass,
}

impl DeviceInfo {
    /// Encode this device as a wrapper-style `Disk` record for `PutDisk`.
    /// Lifecycle fields (`pool_uuid`, `state`, `generation`) are owned by
    /// meta and are left at their zero values; the dataplane only reports
    /// hardware facts and `present=true`.
    pub fn to_disk(&self, node: &str) -> Disk {
        Disk {
            uuid: self.wwn.clone(),
            node: node.to_string(),
            device_path: format!("/dev/{}", self.slot),
            size_bytes: self.size_bytes,
            pool_uuid: String::new(),
            state: String::new(),
            tier: self.class.as_str().to_string(),
            present: true,
            generation: 0,
        }
    }
}

/// Scan the configured sysfs root and return one `DeviceInfo` per disk.
///
/// On platforms where `/sys/block` does not exist (macOS, *BSD), returns
/// an empty vector — discovery is a Linux-only concept and the caller is
/// expected to tolerate that.
pub fn discover() -> Result<Vec<DeviceInfo>> {
    discover_in(Path::new(DEFAULT_SYSFS_ROOT))
}

/// Scan a custom sysfs root. Tests use this to inject a fake tree.
pub fn discover_in(sysfs_root: &Path) -> Result<Vec<DeviceInfo>> {
    if !sysfs_root.exists() {
        return Ok(Vec::new());
    }
    let mut out = Vec::new();
    let entries = std::fs::read_dir(sysfs_root).map_err(|e| {
        DataPlaneError::IoError(std::io::Error::new(
            e.kind(),
            format!("read_dir {}: {e}", sysfs_root.display()),
        ))
    })?;
    for entry in entries.flatten() {
        let slot_path = entry.path();
        let slot = match slot_path.file_name().and_then(|s| s.to_str()) {
            Some(s) => s.to_string(),
            None => continue,
        };
        if is_partition_or_virtual(&slot) {
            continue;
        }
        match probe_device(&slot_path, &slot) {
            Ok(info) => out.push(info),
            Err(e) => log::debug!("disk_discovery: skipping {slot}: {e}"),
        }
    }
    out.sort_by(|a, b| a.slot.cmp(&b.slot));
    Ok(out)
}

/// Filter common sysfs entries that aren't user-visible disks.
fn is_partition_or_virtual(slot: &str) -> bool {
    // Skip loop, ram, dm-, md*, zram*, sr* (optical).
    matches!(
        slot,
        s if s.starts_with("loop")
            || s.starts_with("ram")
            || s.starts_with("dm-")
            || s.starts_with("md")
            || s.starts_with("zram")
            || s.starts_with("sr")
    )
}

fn probe_device(slot_path: &Path, slot: &str) -> Result<DeviceInfo> {
    let size_sectors: u64 = read_u64(&slot_path.join("size")).unwrap_or(0);
    let size_bytes = size_sectors.saturating_mul(512);

    if size_bytes == 0 {
        return Err(DataPlaneError::BdevError(format!(
            "{slot}: size=0, treating as not a disk"
        )));
    }

    let rotational: u64 = read_u64(&slot_path.join("queue").join("rotational")).unwrap_or(1);
    let discard_max: u64 =
        read_u64(&slot_path.join("queue").join("discard_max_bytes")).unwrap_or(0);

    let class = classify(slot, rotational, discard_max);

    let device_dir = slot_path.join("device");
    let vendor = read_trim(&device_dir.join("vendor")).unwrap_or_default();
    let model = read_trim(&device_dir.join("model")).unwrap_or_default();
    let serial = read_trim(&device_dir.join("serial"))
        .or_else(|| read_trim(&device_dir.join("wwid")))
        .unwrap_or_default();
    let wwn = read_trim(&device_dir.join("wwid"))
        .filter(|s| !s.is_empty())
        .unwrap_or_else(|| {
            if !serial.is_empty() {
                format!("{slot}:{serial}")
            } else {
                slot.to_string()
            }
        });

    Ok(DeviceInfo {
        slot: slot.to_string(),
        wwn,
        vendor,
        model,
        serial,
        size_bytes,
        class,
    })
}

fn classify(slot: &str, rotational: u64, discard_max: u64) -> DeviceClass {
    if slot.starts_with("nvme") {
        return DeviceClass::Nvme;
    }
    if rotational == 1 {
        return DeviceClass::Hdd;
    }
    // rotational == 0 && (discard_max > 0 || SATA SSD): SSD class.
    let _ = discard_max;
    DeviceClass::Ssd
}

fn read_u64(path: &Path) -> Option<u64> {
    read_trim(path).and_then(|s| s.parse::<u64>().ok())
}

fn read_trim(path: &Path) -> Option<String> {
    std::fs::read_to_string(path)
        .ok()
        .map(|s| s.trim().to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn fake_sysfs(root: &Path, slot: &str, fields: &[(&str, &str)]) {
        let dir = root.join(slot);
        std::fs::create_dir_all(dir.join("queue")).unwrap();
        std::fs::create_dir_all(dir.join("device")).unwrap();
        for (k, v) in fields {
            let p = dir.join(k);
            if let Some(parent) = p.parent() {
                std::fs::create_dir_all(parent).unwrap();
            }
            std::fs::write(p, v).unwrap();
        }
    }

    #[test]
    fn classify_nvme_by_slot_name() {
        assert_eq!(classify("nvme0n1", 0, 0), DeviceClass::Nvme);
        assert_eq!(classify("nvme9n2", 1, 0), DeviceClass::Nvme);
    }

    #[test]
    fn classify_hdd_when_rotational() {
        assert_eq!(classify("sda", 1, 0), DeviceClass::Hdd);
    }

    #[test]
    fn classify_ssd_when_non_rotational() {
        assert_eq!(classify("sdb", 0, 4096), DeviceClass::Ssd);
    }

    #[test]
    fn discover_empty_root_is_ok() {
        let dir = tempfile::tempdir().unwrap();
        let infos = discover_in(dir.path()).unwrap();
        assert!(infos.is_empty());
    }

    #[test]
    fn discover_skips_loop_and_ram() {
        let dir = tempfile::tempdir().unwrap();
        fake_sysfs(
            dir.path(),
            "loop0",
            &[("size", "100"), ("queue/rotational", "0")],
        );
        fake_sysfs(
            dir.path(),
            "ram0",
            &[("size", "100"), ("queue/rotational", "0")],
        );
        let infos = discover_in(dir.path()).unwrap();
        assert!(infos.is_empty());
    }

    #[test]
    fn discover_finds_sata_disk() {
        let dir = tempfile::tempdir().unwrap();
        fake_sysfs(
            dir.path(),
            "sda",
            &[
                ("size", "1953525168"), // 1 TB / 512
                ("queue/rotational", "1"),
                ("queue/discard_max_bytes", "0"),
                ("device/model", "WDC WD10EZEX-08W"),
                ("device/vendor", "ATA"),
                ("device/serial", "WD-WCC1234"),
            ],
        );
        let infos = discover_in(dir.path()).unwrap();
        assert_eq!(infos.len(), 1);
        let d = &infos[0];
        assert_eq!(d.slot, "sda");
        assert_eq!(d.class, DeviceClass::Hdd);
        assert_eq!(d.size_bytes, 1953525168 * 512);
        assert_eq!(d.model, "WDC WD10EZEX-08W");
        assert_eq!(d.serial, "WD-WCC1234");
        assert_eq!(d.wwn, "sda:WD-WCC1234");
    }

    #[test]
    fn discover_finds_nvme_with_wwid() {
        let dir = tempfile::tempdir().unwrap();
        fake_sysfs(
            dir.path(),
            "nvme0n1",
            &[
                ("size", "1024000"),
                ("queue/rotational", "0"),
                ("queue/discard_max_bytes", "2147483648"),
                ("device/model", "Samsung 980 PRO"),
                ("device/wwid", "naa.5002538f31201234"),
            ],
        );
        let infos = discover_in(dir.path()).unwrap();
        assert_eq!(infos.len(), 1);
        let d = &infos[0];
        assert_eq!(d.slot, "nvme0n1");
        assert_eq!(d.class, DeviceClass::Nvme);
        assert_eq!(d.wwn, "naa.5002538f31201234");
    }

    #[test]
    fn discover_returns_sorted_results() {
        let dir = tempfile::tempdir().unwrap();
        for slot in ["sdc", "sda", "sdb"] {
            fake_sysfs(
                dir.path(),
                slot,
                &[("size", "1000"), ("queue/rotational", "1")],
            );
        }
        let infos = discover_in(dir.path()).unwrap();
        let slots: Vec<_> = infos.iter().map(|d| d.slot.clone()).collect();
        assert_eq!(slots, vec!["sda", "sdb", "sdc"]);
    }

    #[test]
    fn to_disk_populates_status_defaults() {
        let info = DeviceInfo {
            slot: "sda".into(),
            wwn: "sda:abc".into(),
            vendor: "ATA".into(),
            model: "WDC".into(),
            serial: "abc".into(),
            size_bytes: 1024,
            class: DeviceClass::Hdd,
        };
        let d = info.to_disk("node-A");
        assert_eq!(d.uuid, "sda:abc");
        assert_eq!(d.node, "node-A");
        assert_eq!(d.device_path, "/dev/sda");
        assert!(d.present);
        assert_eq!(d.size_bytes, 1024);
        assert_eq!(d.tier, "hdd");
    }
}
