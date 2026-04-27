//! Volume ↔ chunk math for the frontend.
//!
//! This is a meta-driven port of `dataplane/src/chunk/engine.rs`. The
//! original engine carried CRUSH placement, multi-replica fan-out, EC
//! encode/decode, and a remote-NDP path. None of those belong in the
//! frontend:
//!
//!  - Placement comes from `novanas-meta` via `GetChunkMap` and is cached
//!    locally (`ChunkMapCache`).
//!  - Replication is the data daemon's job (cross-disk on the same host).
//!    The frontend issues exactly one PutChunk per chunk; data fans it
//!    out across local disks per the protection spec.
//!  - There is no remote NDP. The frontend is the local I/O frontend on
//!    the same host as data; NDP is the UDS protocol between them.
//!
//! What this module owns:
//!  - Volume offset → chunk index math.
//!  - Splitting a write buffer into chunk-sized slices.
//!  - Assembling a read by concatenating chunk payloads in order.
//!  - SHA-256 content-addressing for new chunks.
//!  - Wiring the write-back cache for sub-block-aligned writes.

use std::sync::Arc;

use ring::digest;

use crate::chunk_map_cache::ChunkMapCache;
use crate::error::{FrontendError, Result};
use crate::ndp_client::NdpChunkClient;
use crate::write_cache::{coalesce_writes, AbsorbResult, ShardedWriteCache};

/// Default chunk size — must match the data daemon (`backend::chunk_store`).
/// 4 MiB is the canonical setting from docs/16.
pub const CHUNK_SIZE: usize = 4 * 1024 * 1024;

/// On-wire chunk header — 16 bytes preceding the chunk payload.
///
/// Layout:
///   0..4   "NVAC" magic
///   4      version (1)
///   5      flags  (reserved, must be 0)
///   6..10  CRC-32C of payload
///   10..14 payload length (u32 LE)
///   14..16 reserved (zero)
#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct ChunkHeader {
    pub magic: [u8; 4],
    pub version: u8,
    pub flags: u8,
    pub checksum: u32,
    pub data_len: u32,
    pub _reserved: [u8; 2],
}

impl ChunkHeader {
    pub const SIZE: usize = 16;

    pub fn to_bytes(&self) -> [u8; Self::SIZE] {
        let mut out = [0u8; Self::SIZE];
        out[0..4].copy_from_slice(&self.magic);
        out[4] = self.version;
        out[5] = self.flags;
        out[6..10].copy_from_slice(&self.checksum.to_le_bytes());
        out[10..14].copy_from_slice(&self.data_len.to_le_bytes());
        out[14..16].copy_from_slice(&self._reserved);
        out
    }

    pub fn from_bytes(buf: &[u8; Self::SIZE]) -> Result<Self> {
        let magic: [u8; 4] = buf[0..4].try_into().unwrap();
        if &magic != b"NVAC" {
            return Err(FrontendError::Chunk(format!(
                "bad chunk magic: {:?}",
                magic
            )));
        }
        Ok(Self {
            magic,
            version: buf[4],
            flags: buf[5],
            checksum: u32::from_le_bytes(buf[6..10].try_into().unwrap()),
            data_len: u32::from_le_bytes(buf[10..14].try_into().unwrap()),
            _reserved: buf[14..16].try_into().unwrap(),
        })
    }
}

/// The frontend's chunk engine. Owns:
///  - the chunk-map cache (placement lookups for a volume offset)
///  - the NDP client (chunk PUT/GET to data over UDS)
///  - the write-back cache for sub-block-sized aligned writes
pub struct ChunkEngine {
    cache: Arc<ChunkMapCache>,
    ndp: Arc<dyn NdpChunkClient>,
    write_cache: ShardedWriteCache,
}

impl ChunkEngine {
    pub fn new(cache: Arc<ChunkMapCache>, ndp: Arc<dyn NdpChunkClient>) -> Self {
        Self {
            cache,
            ndp,
            write_cache: ShardedWriteCache::new(),
        }
    }

    /// SHA-256 hex digest of `data`. Used as the chunk_id for new chunks.
    pub fn compute_chunk_id(data: &[u8]) -> String {
        let d = digest::digest(&digest::SHA256, data);
        hex::encode(d.as_ref())
    }

    pub fn prepare_chunk(data: &[u8]) -> Vec<u8> {
        let header = ChunkHeader {
            magic: *b"NVAC",
            version: 1,
            flags: 0,
            checksum: crc32c::crc32c(data),
            data_len: data.len() as u32,
            _reserved: [0; 2],
        };
        let mut buf = Vec::with_capacity(ChunkHeader::SIZE + data.len());
        buf.extend_from_slice(&header.to_bytes());
        buf.extend_from_slice(data);
        buf
    }

    /// Verify a chunk-with-header buffer's CRC-32C; return the payload slice
    /// length on success.
    pub fn verify_chunk(buf: &[u8]) -> Result<usize> {
        if buf.len() < ChunkHeader::SIZE {
            return Err(FrontendError::Chunk("chunk too small".into()));
        }
        let header_bytes: [u8; ChunkHeader::SIZE] = buf[..ChunkHeader::SIZE]
            .try_into()
            .map_err(|_| FrontendError::Chunk("header read failed".into()))?;
        let header = ChunkHeader::from_bytes(&header_bytes)?;
        let data_len = header.data_len as usize;
        if buf.len() < ChunkHeader::SIZE + data_len {
            return Err(FrontendError::Chunk("chunk truncated".into()));
        }
        let payload = &buf[ChunkHeader::SIZE..ChunkHeader::SIZE + data_len];
        let actual = crc32c::crc32c(payload);
        if header.checksum != actual {
            return Err(FrontendError::Chunk(format!(
                "CRC mismatch: stored={:#010x}, actual={:#010x}",
                header.checksum, actual
            )));
        }
        Ok(data_len)
    }

    /// Split a slice into `CHUNK_SIZE`-sized pieces. The last piece may be
    /// shorter than `CHUNK_SIZE`.
    pub fn split_into_chunks(data: &[u8]) -> Vec<&[u8]> {
        data.chunks(CHUNK_SIZE).collect()
    }

    // -----------------------------------------------------------------------
    // Whole-volume read/write (used by full-block I/O paths and tests)
    // -----------------------------------------------------------------------

    /// Issue a write to data for the whole `data` buffer at `offset`, by
    /// chunking it and PUT-ing each piece. Returns the chunk_ids that were
    /// produced, in chunk-index order. Existing chunks at the affected
    /// indices are NOT freed by this path — that is the data daemon's
    /// problem (refcounting via meta).
    pub async fn write_volume(
        &self,
        volume_name: &str,
        offset: u64,
        data: &[u8],
    ) -> Result<Vec<String>> {
        if !(offset as usize).is_multiple_of(CHUNK_SIZE) {
            return Err(FrontendError::Chunk(format!(
                "write_volume: offset {} not chunk-aligned",
                offset
            )));
        }
        let start_chunk_index = offset / CHUNK_SIZE as u64;
        let chunks = Self::split_into_chunks(data);
        let mut ids = Vec::with_capacity(chunks.len());

        for (i, raw) in chunks.iter().enumerate() {
            let chunk_index = start_chunk_index + i as u64;
            let chunk_id = Self::compute_chunk_id(raw);
            let prepared = Self::prepare_chunk(raw);
            self.ndp.put_chunk(&chunk_id, &prepared).await?;
            // Record the freshly-written chunk's id in the cache so the
            // very next read sees it without round-tripping to meta.
            self.cache
                .record_chunk_id(volume_name, chunk_index, &chunk_id);
            ids.push(chunk_id);
        }
        Ok(ids)
    }

    /// Read `length` bytes starting at `offset` for `volume_name`. Looks up
    /// chunk_ids via the cache (and meta on miss), GETs each chunk, verifies
    /// CRC-32C, and assembles the result.
    pub async fn read_volume(
        &self,
        volume_name: &str,
        offset: u64,
        length: u64,
    ) -> Result<Vec<u8>> {
        if length == 0 {
            return Ok(Vec::new());
        }
        let first_chunk = offset / CHUNK_SIZE as u64;
        let last_chunk = (offset + length - 1) / CHUNK_SIZE as u64;
        let mut out = Vec::with_capacity(length as usize);

        for ci in first_chunk..=last_chunk {
            let cid = self
                .cache
                .lookup_or_fetch(volume_name, ci)
                .await?
                .ok_or_else(|| {
                    FrontendError::Chunk(format!(
                        "no chunk_id for vol={} chunk_index={}",
                        volume_name, ci
                    ))
                })?;
            let buf = self.ndp.get_chunk(&cid).await?;
            let payload_len = Self::verify_chunk(&buf)?;
            let chunk_payload = &buf[ChunkHeader::SIZE..ChunkHeader::SIZE + payload_len];

            let chunk_byte_start = ci * CHUNK_SIZE as u64;
            let lo = if ci == first_chunk {
                (offset - chunk_byte_start) as usize
            } else {
                0
            };
            let hi = if ci == last_chunk {
                ((offset + length) - chunk_byte_start) as usize
            } else {
                CHUNK_SIZE
            };
            let hi = hi.min(chunk_payload.len());
            if lo > chunk_payload.len() {
                return Err(FrontendError::Chunk(format!(
                    "chunk {} payload too short for offset {}",
                    cid, offset
                )));
            }
            out.extend_from_slice(&chunk_payload[lo..hi]);
        }
        Ok(out)
    }

    // -----------------------------------------------------------------------
    // Sub-block path (write-back cache + flush)
    // -----------------------------------------------------------------------

    /// Absorb a sub-block aligned write into the cache, or pass through.
    /// On overflow, contiguous absorbed sub-blocks are coalesced and
    /// flushed via `flush_single_write`.
    pub async fn sub_block_write(&self, volume_name: &str, offset: u64, data: &[u8]) -> Result<()> {
        match self.write_cache.absorb(volume_name, offset, data) {
            AbsorbResult::Cached => {
                let shard = self.write_cache.shard_index_for(volume_name, offset);
                if self.write_cache.shard_needs_flush(shard) {
                    let overflow = self.write_cache.drain_shard_overflow(shard);
                    if !overflow.is_empty() {
                        let coalesced = coalesce_writes(overflow);
                        for cw in coalesced {
                            if let Err(e) = self
                                .flush_single_write(&cw.volume_id, cw.offset, &cw.data)
                                .await
                            {
                                log::warn!(
                                    "overflow flush failed vol={} offset={}: {}",
                                    cw.volume_id,
                                    cw.offset,
                                    e
                                );
                                self.write_cache.absorb(&cw.volume_id, cw.offset, &cw.data);
                            }
                        }
                    }
                }
                Ok(())
            }
            AbsorbResult::NotAligned => {
                self.write_cache
                    .invalidate_range(volume_name, offset, data.len() as u64);
                self.flush_single_write(volume_name, offset, data).await
            }
            AbsorbResult::Full => self.flush_single_write(volume_name, offset, data).await,
        }
    }

    /// Read `length` bytes from the volume; checks the write-back cache
    /// first so unflushed sub-blocks shadow on-disk content.
    pub async fn sub_block_read(
        &self,
        volume_name: &str,
        offset: u64,
        length: u64,
    ) -> Result<Vec<u8>> {
        if let Some(data) = self.write_cache.lookup(volume_name, offset, length) {
            return Ok(data);
        }
        self.read_volume(volume_name, offset, length).await
    }

    /// Flush a single (possibly cross-chunk) write to the data daemon.
    ///
    /// For each chunk this write touches we read the existing chunk (if
    /// any), splice in the new bytes, recompute the chunk_id, PUT the new
    /// chunk, and update the chunk-map cache. Two design notes:
    ///
    ///  * The very first write to an empty chunk skips the read step. We
    ///    detect this by `cache.lookup_or_fetch` returning `None`.
    ///  * Updates on existing chunks are immutable: a new chunk_id is
    ///    produced. Old-chunk garbage collection is meta+data's job.
    pub async fn flush_single_write(
        &self,
        volume_name: &str,
        offset: u64,
        data: &[u8],
    ) -> Result<()> {
        if data.is_empty() {
            return Ok(());
        }
        let first_chunk = offset / CHUNK_SIZE as u64;
        let last_chunk = (offset + data.len() as u64 - 1) / CHUNK_SIZE as u64;

        let mut written = 0usize;
        for ci in first_chunk..=last_chunk {
            let chunk_byte_start = ci * CHUNK_SIZE as u64;

            // Existing chunk content (or all-zeros for an empty chunk).
            let existing: Vec<u8> = match self.cache.lookup_or_fetch(volume_name, ci).await? {
                Some(cid) => {
                    let buf = self.ndp.get_chunk(&cid).await?;
                    let plen = Self::verify_chunk(&buf)?;
                    buf[ChunkHeader::SIZE..ChunkHeader::SIZE + plen].to_vec()
                }
                None => vec![0u8; CHUNK_SIZE],
            };

            let mut payload = if existing.len() < CHUNK_SIZE {
                let mut v = existing;
                v.resize(CHUNK_SIZE, 0);
                v
            } else {
                existing
            };

            let lo = if ci == first_chunk {
                (offset - chunk_byte_start) as usize
            } else {
                0
            };
            let chunk_data_remaining = data.len() - written;
            let copy_len = (CHUNK_SIZE - lo).min(chunk_data_remaining);
            payload[lo..lo + copy_len].copy_from_slice(&data[written..written + copy_len]);
            written += copy_len;

            let chunk_id = Self::compute_chunk_id(&payload);
            let prepared = Self::prepare_chunk(&payload);
            self.ndp.put_chunk(&chunk_id, &prepared).await?;
            self.cache.record_chunk_id(volume_name, ci, &chunk_id);
        }
        Ok(())
    }

    /// Drain and persist all cached sub-blocks for a volume.
    pub async fn flush(&self, volume_name: &str) -> Result<()> {
        let entries = self.write_cache.drain_volume(volume_name);
        if entries.is_empty() {
            return Ok(());
        }
        let raw: Vec<(String, u64, Vec<u8>)> = entries
            .into_iter()
            .map(|(off, data)| (volume_name.to_string(), off, data))
            .collect();
        let coalesced = coalesce_writes(raw);
        for cw in coalesced {
            self.flush_single_write(&cw.volume_id, cw.offset, &cw.data)
                .await?;
        }
        Ok(())
    }

    /// Force a sub-block-aligned slice into the cache (test hook).
    pub fn write_cache_absorb_test_only(
        &self,
        volume_name: &str,
        offset: u64,
        data: &[u8],
    ) -> AbsorbResult {
        self.write_cache.absorb(volume_name, offset, data)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::chunk_map_cache::ChunkMapCache;
    use crate::ndp_client::tests::FakeNdp;
    use crate::ndp_client::NdpChunkClient;
    use crate::write_cache::SUB_BLOCK_SIZE;

    fn make_engine() -> (Arc<FakeNdp>, ChunkEngine) {
        let ndp = Arc::new(FakeNdp::new());
        let cache = Arc::new(ChunkMapCache::in_memory());
        let engine = ChunkEngine::new(cache, ndp.clone() as Arc<dyn NdpChunkClient>);
        (ndp, engine)
    }

    #[test]
    fn content_id_is_deterministic() {
        let id1 = ChunkEngine::compute_chunk_id(b"hello");
        let id2 = ChunkEngine::compute_chunk_id(b"hello");
        assert_eq!(id1, id2);
        assert_eq!(id1.len(), 64);
    }

    #[test]
    fn prepare_and_verify_chunk_roundtrip() {
        let payload = b"the quick brown fox";
        let prepared = ChunkEngine::prepare_chunk(payload);
        let len = ChunkEngine::verify_chunk(&prepared).unwrap();
        assert_eq!(len, payload.len());
        let mut bad = prepared.clone();
        let last = bad.len() - 1;
        bad[last] ^= 0xFF;
        assert!(ChunkEngine::verify_chunk(&bad).is_err());
    }

    #[test]
    fn split_chunks_handles_remainder() {
        let data = vec![0u8; 10 * 1024 * 1024];
        let pieces = ChunkEngine::split_into_chunks(&data);
        assert_eq!(pieces.len(), 3);
        assert_eq!(pieces[0].len(), CHUNK_SIZE);
        assert_eq!(pieces[1].len(), CHUNK_SIZE);
        assert_eq!(pieces[2].len(), 2 * 1024 * 1024);
    }

    #[tokio::test]
    async fn write_then_read_full_chunks() {
        let (_ndp, engine) = make_engine();
        let data = vec![0x42u8; 8 * 1024 * 1024];
        let ids = engine.write_volume("vol-A", 0, &data).await.unwrap();
        assert_eq!(ids.len(), 2);
        let got = engine
            .read_volume("vol-A", 0, data.len() as u64)
            .await
            .unwrap();
        assert_eq!(got, data);
    }

    #[tokio::test]
    async fn read_partial_within_a_chunk() {
        let (_ndp, engine) = make_engine();
        let mut data = vec![0u8; CHUNK_SIZE];
        data[1024..1028].copy_from_slice(&[0xDE, 0xAD, 0xBE, 0xEF]);
        engine.write_volume("vol-B", 0, &data).await.unwrap();
        let got = engine.read_volume("vol-B", 1024, 4).await.unwrap();
        assert_eq!(got, vec![0xDE, 0xAD, 0xBE, 0xEF]);
    }

    #[tokio::test]
    async fn read_spans_chunk_boundary() {
        let (_ndp, engine) = make_engine();
        let mut data = vec![0u8; 2 * CHUNK_SIZE];
        for (i, b) in data.iter_mut().enumerate() {
            *b = (i % 251) as u8;
        }
        engine.write_volume("vol-C", 0, &data).await.unwrap();
        // Read 1 KiB straddling the chunk boundary.
        let lo = CHUNK_SIZE as u64 - 512;
        let got = engine.read_volume("vol-C", lo, 1024).await.unwrap();
        assert_eq!(got, &data[lo as usize..lo as usize + 1024]);
    }

    #[tokio::test]
    async fn sub_block_aligned_write_is_absorbed_and_flushed() {
        let (ndp, engine) = make_engine();
        let sb = vec![0xCDu8; SUB_BLOCK_SIZE as usize];
        engine.sub_block_write("vol-D", 0, &sb).await.unwrap();
        // Read should see the pending sub-block (cache hit).
        let got = engine.sub_block_read("vol-D", 0, 4096).await.unwrap();
        assert_eq!(got, vec![0xCDu8; 4096]);
        // After flush the chunk should exist on the fake NDP.
        engine.flush("vol-D").await.unwrap();
        assert!(ndp.put_count() >= 1);
    }

    #[tokio::test]
    async fn unaligned_write_passes_through() {
        let (ndp, engine) = make_engine();
        // Misaligned offset (must hit the NotAligned arm).
        let buf = vec![0xAAu8; 4096];
        engine.sub_block_write("vol-E", 1024, &buf).await.unwrap();
        // The bypass path issues a flush_single_write directly, which
        // PUTs at least one chunk to the data daemon.
        assert!(ndp.put_count() >= 1);
    }
}
