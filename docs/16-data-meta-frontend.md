# 16 — Data / Meta / Frontend (the Rust storage rewrite)

> **Status: locked architecture; rebuild in progress.**
> This document supersedes the storage portions of 01-architecture-overview.md.
> Anything in 01 about Go agent/controller, NovaStor scaleout, multi-host
> replication, or the lvm/file/uring backends is obsolete.

## Why this doc exists

The previous storage stack was inherited from a multi-host scaleout product (NovaStor) and partially repurposed for NovaNas's single-node design. The result was three half-coherent code paths:

- A Go agent + Go controller that reconciled k8s CRDs.
- A Rust SPDK dataplane that did the actual I/O but was steered by the Go agent.
- A Go metadata service backed by Badger.

The simplification: **all storage code is Rust**, **single host**, **no CRDs**, three daemons with strict roles. This is the Ceph parallel applied to a single appliance: OSD-equivalent + MON-equivalent + RGW/RBD-gateway-equivalent.

## The three daemons

```
external client (NVMe-oF / iSCSI initiator)
        │
        ▼
┌────────────────────────────────────────────────┐
│  novanas-frontend  (Rust, SPDK)                 │  HOT PATH
│                                                  │
│   protocol target — NVMe-oF / iSCSI / future S3 │
│        │                                          │
│        ▼                                          │
│   volume bdev — the SPDK bdev representing       │
│   each user-facing volume                        │
│        │                                          │
│   on read(volume_id, offset, len):               │
│     ① ask META: chunks for [offset, offset+len)  │
│     ② issue GetChunk over NDP/UDS to DATA        │
│     ③ assemble + return                          │
│   on write(volume_id, offset, buf):              │
│     ① split into chunks                          │
│     ② issue PutChunk(chunk_id, payload)          │
│     ③ ack                                         │
│                                                  │
│   chooses the exposure type:                     │
│     · block: NVMe-oF / iSCSI (current scope)     │
│     · object: S3 (future)                        │
└──────────┬──────────┬───────────────────────────┘
           │ NDP/UDS   │ control gRPC
           ▼           ▼
┌────────────────┐  ┌──────────────────────────────┐
│ novanas-data   │  │ novanas-meta  (Rust, redb)   │  COLD PATH
│ (Rust, SPDK)   │  │                                │
│                 │  │  chunk map                     │
│  chunk engine   │  │   chunk_id → [disk_id × N]    │
│   PutChunk      │  │  pool registry (disk → pool)   │
│   GetChunk      │  │  CRUSH (placement)             │
│   DeleteChunk   │  │  policy CHECKER                │
│                 │  │   (compares state vs spec,     │
│  per-disk       │◄─┤    emits move/replicate/scrub  │
│  chunk stores   │  │    commands to data)           │
│  bdev mgmt      │  │  scheduler                     │
│  (NVMe vfio,    │  │                                │
│   SATA AIO)     │  │  bootstrap-from-superblock     │
│  superblock R/W │  │   (assembles its own metadata  │
│  policy MOVER   │  │    volume from disk superblocks│
│   (executes     │  │    on first start)             │
│    meta's       │  │                                │
│    commands)    │  │                                │
└────────────────┘  └──────────────────────────────┘
                                 ▲
                                 │ subscribe
                                 │
┌────────────────────────────────┴───────────────┐
│  novanas-api  (TypeScript, Postgres)            │  user CRUD
│   pools, disks, volumes, policies               │
└─────────────────────────────────────────────────┘
```

### Daemon responsibilities

**`novanas-data`** — owns disks and chunks.
- Per-host daemon. Runs SPDK.
- Attaches local disks: NVMe via `vfio-pci` (kernel-bypass), SATA via SPDK's AIO bdev.
  No `uring`, no `loop`.
- Per-disk chunk stores. Chunks are immutable, content-addressed, 4 MiB by default.
- Per-disk superblock at LBA 0 + last LBA. Carries `pool_uuid`, `disk_uuid`, `role`, CRUSH digest, metadata-volume locator. CRC32C protected. Format already implemented in `storage/dataplane/src/backend/superblock.rs`.
- Replication is **across local disks within this daemon**. Repl factor `N` = `N` distinct local disks per chunk picked by CRUSH (failure domain = disk). There is **no peer-to-peer replication, no cross-host plumbing**.
- Policy **mover**: executes commands from meta — `replicate(chunk, src_disk, dst_disk)`, `migrate(chunk, src_pool, dst_pool)`, `scrub(chunk)`.
- Exposes a chunk service on Unix domain socket (`/var/run/novanas/ndp.sock`).

**`novanas-meta`** — single-instance brain, no I/O.
- Authoritative chunk map, pool registry, disk registry, policy specs.
- CRUSH placement (straw2). Failure domain = disk only.
- Policy **checker**: scans the chunk map vs. each volume's protection spec, emits commands to data's mover when state diverges.
- Persistent store: **redb** (single-file, transactional, pure-Rust).
- Subscribes to the API server for `Pool`, `Disk`, `BlockVolume`, etc. changes.
- Runs the bootstrap-from-superblock flow on first start: scans data's reported disk superblocks, agrees on a CRUSH-map digest, locates the metadata volume, mounts it via the dataplane, and opens its embedded store on top.

**`novanas-frontend`** — protocol surface, hot-path I/O.
- Per-host daemon. Runs SPDK.
- Hosts the SPDK NVMe-oF target (and later iSCSI / S3 / NFS+SMB through vhost-blk + ganesha/samba — out of scope today).
- For each `BlockVolume`: builds a custom SPDK bdev whose I/O internals fan out to data's chunk service via NDP-over-UDS, using the chunk map fetched from meta.
- The volume bdev does the volume-offset → chunk math; data's chunk engine does the chunk-level work; meta does CRUSH + policy.
- Decides the *exposure type*: today block (NVMe-oF/iSCSI), in future object (S3) — same chunks, different surface.
- All client I/O traverses this daemon. Latency-critical, polling-mode SPDK reactor.

**`novanas-api`** (unchanged) — TypeScript Fastify, Postgres-backed. User-facing CRUD only. Knows nothing about chunks; just stores intent.

## Hard rules (these are guard rails)

1. **All hot-path code is Rust.** Frontend and data are Rust. Anything that sits in a per-I/O path stays in Rust.
2. **Single host.** No multi-host clustering. No peer-to-peer chunk replication. No NDP-over-network. NDP is the chunk service protocol, but it runs over Unix-domain sockets between local SPDK processes.
3. **No Kubernetes assumption.** The daemons are systemd services on the appliance host. Containers and helm are *one* deployment path; bare-metal is the primary.
4. **Volume = collection of chunks**, not a disk. Disks are members of pools; volumes are striped across chunks; the bdev a client sees is assembled at the frontend.
5. **Block exposure first.** NVMe-oF target is the only protocol in scope today. iSCSI, NFS/SMB (via local vhost-blk + ganesha/samba), and S3 are deferred.
6. **No CRDs anywhere.** API server is the sole source of truth. Meta subscribes to API; data follows meta. No `Reconcile()` against a kube-apiserver. No `BackendAssignment`, no `StoragePoolReconciler`.
7. **One backend type: `raw`.** SPDK driver is chosen by the data daemon based on the disk class (NVMe → vfio-pci, SATA → AIO). The legacy `lvm` and `file` backend types are deleted.

## Existing code → daemon mapping

The current `storage/dataplane/` (Rust) already contains roughly the right code, just fused into one process and tangled with a Go agent that's about to disappear. The split is mostly a matter of moving modules between binaries.

| Module today | Goes to |
|---|---|
| `bdev/novanas_bdev.rs` (the volume bdev, registers as `novanas_<volume_id>`) | **frontend** |
| `chunk/engine.rs` (volume↔chunk math, write cache wiring) | **frontend** |
| `chunk/write_cache.rs`, `chunk/open_chunk.rs`, `chunk/ndp_pool.rs` | **frontend** |
| `spdk/nvmf_manager.rs` (NVMe-oF target) | **frontend** |
| `backend/chunk_store.rs`, `backend/raw_disk.rs` | **data** |
| `backend/superblock.rs` | **data** |
| `chunk/sync.rs`, `bdev/replica.rs`, `bdev/novanas_replica_bdev.rs` (cross-disk replica fan-out) | **data** |
| `chunk/reactor_ndp.rs`, `transport/ndp_server.rs` (NDP UDS server) | **data** |
| `spdk/bdev_manager.rs`, `spdk/env.rs`, `spdk/reactor_dispatch.rs` | both (each runs SPDK) |
| `metadata/crush.rs`, `metadata/types.rs`, `metadata/topology.rs`, `metadata/store.rs`, `metadata/shard.rs` | **meta** (Rust building blocks already exist) |
| `metadata/raft_store.rs`, `metadata/raft_types.rs` | **deleted** (multi-host scaleout) |
| `policy/engine.rs`, `policy/evaluator.rs`, `policy/location_store.rs` | **meta** (the *checker*); **data** keeps `policy/operations.rs` (the *mover*) |
| `backend/lvm.rs`, `backend/file_store.rs` | **deleted** |

### Go code (all going away)

| Today | Action |
|---|---|
| `storage/cmd/agent/`, `storage/internal/agent/` | delete; replaced by data daemon's own per-disk reconciler |
| `storage/cmd/controller/`, `storage/internal/controller/` | delete; replaced by meta subscribing to API |
| `storage/cmd/meta/`, `storage/internal/metadata/` | delete; rewrite in Rust on top of `dataplane/src/metadata/*` building blocks |
| `storage/internal/policy/` (Go checkers) | delete after porting checker logic to Rust meta |
| `storage/internal/disk/` (Go superblock R/W + discovery) | delete; superblock R/W already in Rust, discovery to be added in Rust data |
| `storage/internal/placement/` (Go CRUSH) | delete; duplicate of `dataplane/src/metadata/crush.rs` |
| `storage/api/v1alpha1/` (CRD types) | delete; we have zero CRDs |
| `storage/cmd/csi`, `cmd/scheduler`, `cmd/webhook`, `cmd/s3gw` | delete; out of scope (k8s-shaped, scaleout) |
| `storage/internal/{csi,scheduler,webhook,s3,operator}` | delete; same |
| `storage/cmd/cli`, `storage/internal/cli` | delete; CLI talked to Go meta directly. Replace with API-server CLI when needed |
| `packages/operators/` | delete; was the Go reconciler home |
| `packages/runtime/` | delete; orphaned once operators is gone |
| `packages/sdk/go-client/` | delete; orphaned |

## On-disk superblock format

(Already implemented; documenting for the record.)

4 KiB block at LBA 0 and at the last 4 KiB of the device. Both copies updated under the same write fence; CRC32C over bytes [0..4092). Little-endian.

| Offset | Size | Field |
|---|---|---|
| 0   | 8   | magic = `NOVANAS\0` |
| 8   | 4   | version (currently 1) |
| 12  | 4   | flags (reserved) |
| 16  | 16  | disk UUID |
| 32  | 32  | pool ID (utf-8, zero-padded) |
| 64  | 4   | role (1 = data, 2 = metadata, 3 = both) |
| 68  | 32  | CRUSH map digest (sha256) |
| 100 | 32  | metadata volume name |
| 132 | 64  | metadata volume root chunk ID |
| 196 | 8   | metadata volume version |
| 204 | 8   | created (unix nanos) |
| 212 | 8   | updated (unix nanos) |
| 220 | 3872 | reserved |
| 4092 | 4  | CRC32C |

The metadata-volume locator is what lets meta bootstrap from chunks: at first start, meta has no Badger/redb file. It asks data which disks have a valid superblock, picks any whose `crush_digest` matches the quorum, reads `metadata_volume_root`, mounts the metadata volume via the chunk engine (it's a `BlockVolume` like any other, just bootstrapped specially), formats it `xfs` if first time, and opens its persistent store on top.

## Volume lifecycle (block scope)

```
1. user POSTs /api/v1/disks/<wwn>  { spec: { pool: "hdd" } }
2. meta sees the change (subscribes to API). Asks data: ClaimDisk(wwn, pool_uuid, role=data)
3. data:
     - verifies disk is empty (or refuses)
     - unbinds kernel NVMe / opens AIO for SATA
     - writes superblock (LBA 0 + last LBA)
     - registers a chunk store on the bdev
4. meta updates the pool's disk set; recomputes placement weights

5. user POSTs /api/v1/block-volumes  { pool: "hdd", size: "100G",
                                       protection: { factor: 3 } }
6. meta runs CRUSH: produces (chunk_id, [disk × 3]) tuples.
7. meta tells data: AllocateChunk for each. Chunks are reserved.
8. meta updates the chunk map; volume.status.phase = Ready

9. frontend sees the new volume. Builds a volume bdev.
10. frontend creates an NVMe-oF subsystem fronting the volume bdev.
11. external client connects to the NVMe-oF target on the host's IP.

12. client writes:
     - lands at frontend's NVMe-oF target
     - frontend's volume bdev splits the write into chunks
     - asks meta (cached) where each chunk lives
     - issues PutChunk(chunk_id, data) over NDP/UDS to data
     - data writes the chunk to all 3 designated disks (replication)
     - acks bubble back to client
```

## What we delete on the way

PR-Strip lands a single deletion-only patch:

- All Go storage code: `storage/{cmd,internal,api/v1alpha1}/*` except the proto definitions and Rust dataplane.
- All k8s controller-runtime imports: gone.
- `packages/{operators,runtime,sdk/go-client}/`.
- Helm templates for the deleted binaries.
- Rust: `backend/{lvm,file_store}.rs`, `metadata/raft_*.rs`, the `Lvm`/`File` enum variants and their RPCs.

After this strip the storage subsystem is just `storage/dataplane/` (Rust SPDK) with a tonne of dead branches removed, plus this doc and the API definitions. PR-MetaPort, PR-PolicyPort, PR-FrontendDaemon, PR-DataConsolidate, PR-DiskClaim, PR-VolumeBlock follow.

## See also

- `docs/02-storage-architecture.md` — user-facing storage concepts (StoragePool, Volume kinds, protection policies). Still accurate at the *what*, this doc covers the *how*.
- `docs/14-decision-log.md` — single-host design decision (S9, S12, S14).
- `storage/dataplane/src/backend/superblock.rs` — canonical Rust impl of the superblock format above.
