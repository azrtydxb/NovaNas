//! Disk superblock (A4-Metadata-As-Chunks, docs/02 "Bootstrap").
//!
//! The superblock is the only non-chunk data on a NovaNas disk. It
//! carries the minimum state required to bootstrap the metadata
//! service without access to any prior metadata: disk identity, pool
//! membership, CRUSH-map digest, and a locator for the metadata
//! BlockVolume's root chunk.
//!
//! Byte layout: identical to the Go implementation in
//! `storage/internal/disk/superblock.go`; see that file for the
//! canonical field-offset table. Endianness: little-endian. Size: 4096
//! bytes. A CRC32C checksum over the first 4092 bytes lives at
//! offset 4092.
//!
//! This Rust module is intentionally pure (no SPDK) so it builds in
//! CI without the data-plane feature set. The raw-disk backend will
//! call `read_superblock` / `write_superblock` against an SPDK bdev in
//! production.

use crc32c::crc32c;
use std::fs::OpenOptions;
use std::io::{Read, Seek, SeekFrom, Write};
use std::path::Path;

/// On-disk superblock size (4 KiB).
pub const SUPERBLOCK_SIZE: usize = 4096;

/// Byte offset of the superblock within a raw device.
pub const SUPERBLOCK_OFFSET: u64 = 0;

/// Current superblock format version.
pub const SUPERBLOCK_VERSION: u32 = 1;

/// Magic bytes identifying a NovaNas superblock.
pub const SUPERBLOCK_MAGIC: [u8; 8] = *b"NOVANAS\0";

// Field offsets — keep in sync with the Go implementation.
const OFF_MAGIC: usize = 0;
const OFF_VERSION: usize = 8;
const OFF_FLAGS: usize = 12;
const OFF_DISK_UUID: usize = 16;
const OFF_POOL_ID: usize = 32;
const OFF_ROLE: usize = 64;
const OFF_CRUSH_DIGEST: usize = 68;
const OFF_META_VOL_NAME: usize = 100;
const OFF_META_VOL_ROOT: usize = 132;
const OFF_META_VOL_VERSION: usize = 196;
const OFF_CREATED: usize = 204;
const OFF_UPDATED: usize = 212;
const OFF_CRC: usize = 4092;

const POOL_ID_LEN: usize = 32;
const META_VOL_NAME_LEN: usize = 32;
const META_VOL_ROOT_LEN: usize = 64;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u32)]
pub enum DiskRole {
    Unknown = 0,
    Data = 1,
    Metadata = 2,
    Both = 3,
}

impl DiskRole {
    pub fn from_u32(v: u32) -> Self {
        match v {
            1 => DiskRole::Data,
            2 => DiskRole::Metadata,
            3 => DiskRole::Both,
            _ => DiskRole::Unknown,
        }
    }
}

#[derive(Debug, Clone)]
pub struct Superblock {
    pub version: u32,
    pub flags: u32,
    pub disk_uuid: [u8; 16],
    pub pool_id: String,
    pub role: DiskRole,
    pub crush_digest: [u8; 32],
    pub meta_volume_name: String,
    pub meta_volume_root_chunk: String,
    pub meta_volume_version: u64,
    pub created_unix_nanos: i64,
    pub updated_unix_nanos: i64,
}

impl Default for Superblock {
    fn default() -> Self {
        Self {
            version: SUPERBLOCK_VERSION,
            flags: 0,
            disk_uuid: [0; 16],
            pool_id: String::new(),
            role: DiskRole::Unknown,
            crush_digest: [0; 32],
            meta_volume_name: String::new(),
            meta_volume_root_chunk: String::new(),
            meta_volume_version: 0,
            created_unix_nanos: 0,
            updated_unix_nanos: 0,
        }
    }
}

#[derive(Debug, thiserror::Error)]
pub enum SuperblockError {
    #[error("superblock magic mismatch")]
    BadMagic,
    #[error("superblock CRC mismatch: got {got:08x} want {want:08x}")]
    BadCrc { got: u32, want: u32 },
    #[error("unsupported superblock version {0}")]
    BadVersion(u32),
    #[error("field {name} exceeds on-disk capacity ({len} > {max})")]
    FieldTooLong {
        name: &'static str,
        len: usize,
        max: usize,
    },
    #[error("buffer size {got} does not match SUPERBLOCK_SIZE {want}")]
    BadSize { got: usize, want: usize },
    #[error("io: {0}")]
    Io(#[from] std::io::Error),
}

fn put_fixed_str(
    buf: &mut [u8],
    offset: usize,
    len: usize,
    s: &str,
    name: &'static str,
) -> Result<(), SuperblockError> {
    if s.len() > len {
        return Err(SuperblockError::FieldTooLong {
            name,
            len: s.len(),
            max: len,
        });
    }
    let slot = &mut buf[offset..offset + len];
    for b in slot.iter_mut() {
        *b = 0;
    }
    slot[..s.len()].copy_from_slice(s.as_bytes());
    Ok(())
}

fn read_fixed_str(buf: &[u8], offset: usize, len: usize) -> String {
    let slot = &buf[offset..offset + len];
    let end = slot.iter().position(|&b| b == 0).unwrap_or(len);
    String::from_utf8_lossy(&slot[..end]).into_owned()
}

impl Superblock {
    /// Serialise to a 4096-byte buffer with CRC populated.
    pub fn marshal(&self) -> Result<[u8; SUPERBLOCK_SIZE], SuperblockError> {
        let mut buf = [0u8; SUPERBLOCK_SIZE];
        buf[OFF_MAGIC..OFF_MAGIC + 8].copy_from_slice(&SUPERBLOCK_MAGIC);
        buf[OFF_VERSION..OFF_VERSION + 4].copy_from_slice(&self.version.to_le_bytes());
        buf[OFF_FLAGS..OFF_FLAGS + 4].copy_from_slice(&self.flags.to_le_bytes());
        buf[OFF_DISK_UUID..OFF_DISK_UUID + 16].copy_from_slice(&self.disk_uuid);
        put_fixed_str(&mut buf, OFF_POOL_ID, POOL_ID_LEN, &self.pool_id, "pool_id")?;
        buf[OFF_ROLE..OFF_ROLE + 4].copy_from_slice(&(self.role as u32).to_le_bytes());
        buf[OFF_CRUSH_DIGEST..OFF_CRUSH_DIGEST + 32].copy_from_slice(&self.crush_digest);
        put_fixed_str(
            &mut buf,
            OFF_META_VOL_NAME,
            META_VOL_NAME_LEN,
            &self.meta_volume_name,
            "meta_volume_name",
        )?;
        put_fixed_str(
            &mut buf,
            OFF_META_VOL_ROOT,
            META_VOL_ROOT_LEN,
            &self.meta_volume_root_chunk,
            "meta_volume_root_chunk",
        )?;
        buf[OFF_META_VOL_VERSION..OFF_META_VOL_VERSION + 8]
            .copy_from_slice(&self.meta_volume_version.to_le_bytes());
        buf[OFF_CREATED..OFF_CREATED + 8].copy_from_slice(&self.created_unix_nanos.to_le_bytes());
        buf[OFF_UPDATED..OFF_UPDATED + 8].copy_from_slice(&self.updated_unix_nanos.to_le_bytes());

        let crc = crc32c(&buf[..OFF_CRC]);
        buf[OFF_CRC..OFF_CRC + 4].copy_from_slice(&crc.to_le_bytes());
        Ok(buf)
    }

    pub fn unmarshal(buf: &[u8]) -> Result<Self, SuperblockError> {
        if buf.len() != SUPERBLOCK_SIZE {
            return Err(SuperblockError::BadSize {
                got: buf.len(),
                want: SUPERBLOCK_SIZE,
            });
        }
        if buf[OFF_MAGIC..OFF_MAGIC + 8] != SUPERBLOCK_MAGIC {
            return Err(SuperblockError::BadMagic);
        }
        let got_crc = u32::from_le_bytes(buf[OFF_CRC..OFF_CRC + 4].try_into().unwrap());
        let want_crc = crc32c(&buf[..OFF_CRC]);
        if got_crc != want_crc {
            return Err(SuperblockError::BadCrc {
                got: got_crc,
                want: want_crc,
            });
        }
        let version = u32::from_le_bytes(buf[OFF_VERSION..OFF_VERSION + 4].try_into().unwrap());
        if version != SUPERBLOCK_VERSION {
            return Err(SuperblockError::BadVersion(version));
        }
        let flags = u32::from_le_bytes(buf[OFF_FLAGS..OFF_FLAGS + 4].try_into().unwrap());
        let mut disk_uuid = [0u8; 16];
        disk_uuid.copy_from_slice(&buf[OFF_DISK_UUID..OFF_DISK_UUID + 16]);
        let pool_id = read_fixed_str(buf, OFF_POOL_ID, POOL_ID_LEN);
        let role = DiskRole::from_u32(u32::from_le_bytes(
            buf[OFF_ROLE..OFF_ROLE + 4].try_into().unwrap(),
        ));
        let mut crush_digest = [0u8; 32];
        crush_digest.copy_from_slice(&buf[OFF_CRUSH_DIGEST..OFF_CRUSH_DIGEST + 32]);
        let meta_volume_name = read_fixed_str(buf, OFF_META_VOL_NAME, META_VOL_NAME_LEN);
        let meta_volume_root_chunk = read_fixed_str(buf, OFF_META_VOL_ROOT, META_VOL_ROOT_LEN);
        let meta_volume_version = u64::from_le_bytes(
            buf[OFF_META_VOL_VERSION..OFF_META_VOL_VERSION + 8]
                .try_into()
                .unwrap(),
        );
        let created_unix_nanos =
            i64::from_le_bytes(buf[OFF_CREATED..OFF_CREATED + 8].try_into().unwrap());
        let updated_unix_nanos =
            i64::from_le_bytes(buf[OFF_UPDATED..OFF_UPDATED + 8].try_into().unwrap());
        Ok(Self {
            version,
            flags,
            disk_uuid,
            pool_id,
            role,
            crush_digest,
            meta_volume_name,
            meta_volume_root_chunk,
            meta_volume_version,
            created_unix_nanos,
            updated_unix_nanos,
        })
    }
}

/// Write the superblock at byte offset SUPERBLOCK_OFFSET on `device`.
pub fn write_superblock(device: impl AsRef<Path>, sb: &Superblock) -> Result<(), SuperblockError> {
    let buf = sb.marshal()?;
    let mut f = OpenOptions::new().write(true).open(device.as_ref())?;
    f.seek(SeekFrom::Start(SUPERBLOCK_OFFSET))?;
    f.write_all(&buf)?;
    f.sync_all()?;
    Ok(())
}

/// Read and validate the superblock at SUPERBLOCK_OFFSET on `device`.
pub fn read_superblock(device: impl AsRef<Path>) -> Result<Superblock, SuperblockError> {
    let mut f = OpenOptions::new().read(true).open(device.as_ref())?;
    f.seek(SeekFrom::Start(SUPERBLOCK_OFFSET))?;
    let mut buf = [0u8; SUPERBLOCK_SIZE];
    f.read_exact(&mut buf)?;
    Superblock::unmarshal(&buf)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs::File;
    use std::io::Write;

    fn sample() -> Superblock {
        let mut sb = Superblock {
            pool_id: "pool-default".into(),
            role: DiskRole::Both,
            meta_volume_name: "meta-vol".into(),
            meta_volume_root_chunk: "abc123".into(),
            meta_volume_version: 42,
            ..Superblock::default()
        };
        for i in 0..16 {
            sb.disk_uuid[i] = (i + 1) as u8;
        }
        for i in 0..32 {
            sb.crush_digest[i] = (i * 3) as u8;
        }
        sb
    }

    #[test]
    fn marshal_roundtrip() {
        let sb = sample();
        let buf = sb.marshal().unwrap();
        assert_eq!(buf.len(), SUPERBLOCK_SIZE);
        let got = Superblock::unmarshal(&buf).unwrap();
        assert_eq!(got.pool_id, sb.pool_id);
        assert_eq!(got.role, sb.role);
        assert_eq!(got.meta_volume_name, sb.meta_volume_name);
        assert_eq!(got.meta_volume_root_chunk, sb.meta_volume_root_chunk);
        assert_eq!(got.meta_volume_version, sb.meta_volume_version);
        assert_eq!(got.disk_uuid, sb.disk_uuid);
        assert_eq!(got.crush_digest, sb.crush_digest);
    }

    #[test]
    fn bad_magic() {
        let sb = sample();
        let mut buf = sb.marshal().unwrap();
        buf[0] ^= 0xff;
        assert!(matches!(
            Superblock::unmarshal(&buf),
            Err(SuperblockError::BadMagic)
        ));
    }

    #[test]
    fn bad_crc() {
        let sb = sample();
        let mut buf = sb.marshal().unwrap();
        buf[OFF_POOL_ID] ^= 0x01;
        assert!(matches!(
            Superblock::unmarshal(&buf),
            Err(SuperblockError::BadCrc { .. })
        ));
    }

    #[test]
    fn field_too_long() {
        let mut sb = sample();
        sb.pool_id = "x".repeat(POOL_ID_LEN + 1);
        assert!(matches!(
            sb.marshal(),
            Err(SuperblockError::FieldTooLong { .. })
        ));
    }

    #[test]
    fn write_read_file() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("fake-disk");
        {
            let mut f = File::create(&path).unwrap();
            f.write_all(&vec![0u8; 64 * 1024]).unwrap();
        }
        let sb = sample();
        write_superblock(&path, &sb).unwrap();
        let got = read_superblock(&path).unwrap();
        assert_eq!(got.pool_id, sb.pool_id);
        assert_eq!(got.meta_volume_root_chunk, sb.meta_volume_root_chunk);
    }
}
