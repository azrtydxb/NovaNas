# 02 — Storage Architecture

## Mental model

Three orthogonal concepts:

- **StoragePool**: a bag of physical disks with a tier label. No protection policy.
- **Volume**: a user-facing storage unit. Three volume kinds, all peers:
  - `BlockVolume` — raw block device (for iSCSI, NVMe-oF)
  - `Dataset` — BlockVolume formatted with a filesystem, mounted, exposable via SMB/NFS
  - `Bucket` — native object storage on chunks
- **Protection, tiering, snapshots, replication, backup, quota, encryption** — attributes of a Volume, not of a Pool.

A pool holds disks. Volumes live on a pool. Chunks back every volume.

## Pools

```
StoragePool (cluster-scoped)
├─ tier: hot | warm | cold        (user-defined label)
├─ deviceFilter: preferredClass   (warn on mismatch, don't block)
├─ recoveryRate: aggressive|balanced|gentle
├─ rebalanceOnAdd: manual         (admin-approved default)
└─ disks: [Disk references]       (admin-assigned)
```

Constraints:

- **Heterogeneous disk sizes within a pool are supported** (CRUSH weights by capacity)
- **Mixed disk performance classes warned but allowed** (e.g., NVMe + HDD — UI flags it)
- **No per-pool protection policy** — protection is per-volume
- **Pool tier is a label**; tiering policies on volumes reference pools by tier/name

## Volume types

### BlockVolume

Raw block device exposed to the chunk engine directly. Used for:
- iSCSI and NVMe-oF targets
- Datasets (formatted with a FS on top)
- KubeVirt VM disks
- App PVCs via CSI

### Dataset

A BlockVolume + filesystem + mount point + default ownership + mutable protection policy.

Formatted with **xfs (default)** or **ext4**. Provides the POSIX substrate for SMB and NFS shares.

Datasets own:
- Snapshots, SnapshotSchedule
- Replication jobs
- Cloud backup jobs
- Quota
- Encryption
- Tiering (hot pool → demote to cold pool)
- Default ACL mode (`posix` or `nfsv4`)

### Bucket

Native S3 object storage. Peer of BlockVolume and Dataset — **a bucket is a volume, not a subdivision of a dataset**.

Owns its own protection, tiering, snapshots, replication, cloud backup, quota, encryption — all independently.

## Protection policies

Applied per-volume, mutable in place. Datamover re-encodes chunks on policy change.

| Mode | Disks needed | Survives |
|---|---|---|
| rep×1 | 1 | 0 (warned) |
| rep×2 | 2 | 1 |
| rep×3 | 3 | 2 |
| EC 2+1 | 3 | 1 |
| EC 2+2 | 4 | 2 |
| EC 4+1 | 5 | 1 |
| EC 4+2 | 6 | 2 |
| EC 6+2 | 8 | 2 |
| EC 8+2 | 10+ | 2 |

**Adaptive default** suggested at volume creation based on available disks in the chosen pool. User override always allowed. Policy can be upgraded/downgraded on a populated volume; datamover re-encodes in the background.

**Write quorum is strict** — all shards must ack before the write returns success. No tail-quorum relaxation in v1.

**Failure domain is the device** (single-node). CRUSH hierarchy is auto-detected:
- `root → device` (flat) when single enclosure
- `root → enclosure → device` (enclosure-aware) when multiple enclosures detected via `/sys/class/enclosure`

## Chunk engine

- **Chunk size**: 4 MB fixed for data volumes; configurable per-pool for metadata volumes (typically 64 KB)
- **Chunk ID**: SHA-256 of plaintext (convergent encryption preserves this property across different DKs)
- **Checksum**: CRC-32C per chunk
- **Immutability**: chunks are immutable once sealed
- **Open-chunk extension**: chunks can be in `Open` state (mutable, append-only, UUID-identified) for the metadata WAL, then sealed when full

## Metadata as chunks

Storage metadata (volume → chunk list, CRUSH map, quotas) itself lives as chunks on a dedicated metadata pool.

- Pool default tier: prefers NVMe, falls back to HDDs
- Chunk size: 64 KB (small)
- Stored via the same chunk engine as data — same protection, scrub, GC
- **No Raft on single-node** — BadgerDB FSM stored directly as chunks; open-chunk WAL semantics handle durability

Bootstrap: each device carries a small **superblock** (~4 KB) with node UUID, CRUSH map chunk ID, and that device's slot. Superblock is the only non-chunk data on disk.

## Tiering

Access-based, v1:

- **Promotion**: chunk accessed N times in M minutes → promote to hot tier
- **Demotion**: chunk not accessed in X days → demote to cold tier
- **Policy per-volume**: `primaryPool: fast`, `demoteTo: cold`, thresholds configurable
- **Datamover** runs as background job in the policy engine; never in data path
- Rate-limited by `recoveryRate` on the source pool

## Encryption

Chunk-level, always-on when enabled per-volume.

- **Cipher**: AES-256-GCM (hardware-accelerated on modern x86/ARM, ~5-10 GB/s per core)
- **Convergent encryption**: chunk key = HMAC(DK, plaintextHash). Same plaintext + same DK → same ciphertext → dedup preserved.
- **Key hierarchy**:
  - Master Key (MK) — TPM-sealed or passphrase-protected, lives in OpenBao Transit
  - Dataset/Bucket Key (DK) — one per volume, wrapped by MK
  - Chunk Encryption Key — derived per-chunk from DK + plaintext hash
- **Cryptographic erase**: destroy DK = erase entire volume in O(1)
- **SSE translation** (S3):
  - SSE-S3 → uses Bucket DK (free, automatic)
  - SSE-KMS → references a `kmsKey` API resource = named DK with its own lifecycle
  - SSE-C → client-supplied key, segregated chunk namespace, no dedup (inherent)

Key material never leaves OpenBao Transit; chunk engine calls unwrap operations, caches DKs in-memory per mount.

## Data protection operations

- **Scrub**: periodic background integrity check (CRC verify, replica compare); driven by `scrubSchedule` API resources, one per pool
- **Rebuild**: on disk failure, immediate re-replication from surviving copies onto other healthy disks; rate-limited by `recoveryRate`
- **Rebalance**: on disk add, optionally migrate chunks to include new disk (admin-approved, default off)
- **Replication**: dataset/bucket-level, snapshot-diff-based, push or pull, to another NovaNas or cloud S3
- **Cloud backup**: pluggable engine (restic / borg / kopia)

## What the chunk engine guarantees

- **Strong durability per protection policy** (write-quorum required)
- **Consistency**: reads after successful writes see the write
- **Crash safety**: open-chunk WAL replay on restart restores in-flight state
- **Scrubbing**: silent bit rot detected and repaired from replicas
- **Cryptographic confidentiality** when encryption is enabled on a volume

## What the chunk engine does NOT do

- Locking (handled by protocol layer: SMB oplocks, NFS locks, S3 is lockless)
- ACL enforcement (filesystem or S3 layer)
- Caching layer (beyond chunk-level read caches in agents)
- Clustering across nodes (out of scope)
