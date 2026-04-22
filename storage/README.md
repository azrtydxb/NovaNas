# NovaNas Storage Engine

This directory contains the NovaNas storage engine. It was originally forked
from the [NovaStor](https://github.com/azrtydxb/novastor) project (Apache 2.0)
and has been absorbed into the NovaNas monorepo — going forward, NovaNas owns
this code. There is no upstream sync.

## Layered architecture

The storage engine follows a strict four-layer architecture:

1. **Presentation layer** — NVMe-oF bdev, S3 gateway (the custom NFS/FUSE
   filer that shipped with NovaStor has been removed; NovaNas uses host
   knfsd plus a Samba-in-a-pod for file access — see
   `docs/03-access-protocols.md`).
2. **Chunk engine** — Content-addressed 4 MB immutable chunks, CRUSH
   placement, replication or Reed-Solomon erasure coding.
3. **Backend engine** — File, LVM, or raw NVMe backends that expose SPDK
   bdevs.
4. **Policy engine** — Control plane that monitors actual vs. desired state
   and triggers repair/rebuild. Never in the data path.

See `docs/02-storage-architecture.md` for details.

## Bootstrap sequence (A4-Metadata-As-Chunks)

NovaNas enforces an "everything is chunks" invariant: the metadata service
itself stores its BadgerDB files on a chunk-backed BlockVolume, not on a
local filesystem. To resolve the chicken-and-egg bootstrap problem, each
disk carries a 4 KiB **superblock** at byte offset 0 — the only non-chunk
data on any NovaNas disk.

Startup flow:

1. **Agent** (`cmd/agent/`) boots, enumerates local block devices,
   reads each device's superblock, and classifies it:
   - `ACTIVE`   — valid superblock; disk participates in its declared pool
   - `IDENTIFIED` — device present but superblock missing or corrupt; admin
     must assign (write a superblock) before use
   - `ERROR`   — I/O failure reading the device
   Each ACTIVE superblock carries: disk UUID, pool ID, role
   (data / metadata / both), CRUSH-map digest, and the metadata-volume
   locator (name, root chunk ID, version).
2. **Agent** reports its superblocks to **Meta** via heartbeat (TODO
   (integration): ReportSuperblocks RPC — proto defined in
   `proto/novanas/metadata/v1/metadata_service.proto`).
3. **Meta** (`cmd/meta/`) waits for at least `--min-metadata-disks`
   metadata-role superblocks (up to `--bootstrap-timeout`), then:
   - verifies all reporting disks agree on a single CRUSH-map digest;
   - selects the highest `meta_volume_version` locator as authoritative;
   - (TODO(integration)) assembles the metadata BlockVolume, exposes
     it as a local block device via the NBD bdev, formats with xfs on
     first boot, and mounts at `--meta-mount-path`;
   - opens BadgerDB on the mount.
4. **Meta** serves gRPC as normal.

Two chunk states support this: the classic immutable content-addressed
`SealedChunk`, and the new mutable, append-only, UUID-identified
`OpenChunk`. Open chunks provide WAL-style low-latency semantics for
BadgerDB's value log; once full (or after an idle timeout) they seal
into content-addressed chunks. See `storage/internal/chunk/open_chunk.go`
and `storage/dataplane/src/chunk/open_chunk.rs`.

The `--data-dir` flag on `cmd/meta` is **deprecated**. It is retained as
a fallback while the chunk-mount step is being wired.

## Encryption

NovaNas encrypts chunk data with AES-256-GCM using a convergent key
derivation. A two-level key hierarchy keeps the Master Key inside
OpenBao Transit (it never leaves), while per-volume Dataset Keys
(DKs) are wrapped with the Master Key and stored in the volume's
CRD spec. On mount, the agent unwraps the DK via OpenBao and caches
it in memory for the lifetime of the mount (zeroised on unmount).

Each chunk key is derived as
`HMAC-SHA-256(DK, "novanas/chunk-key/v1" || SHA-256(plaintext))` and
the 96-bit IV is derived with a distinct domain-separation prefix.
Because the derivation is deterministic in `(DK, plaintext)`, the
ciphertext is deterministic too — so dedup over `SHA-256(ciphertext||tag)`
still works within a DK scope, while different DKs never cross-dedup.

SSE-C (customer-supplied-key) objects live in a segregated non-dedup
namespace (random per-chunk IV, "ssec:" chunk-id prefix); full S3
SSE-C wiring is deferred to Wave 5.

Encryption is **off by default** in v1 and opt-in per volume via
`spec.encryption.enabled` on `BlockVolume`, `SharedFilesystem`, and
`ObjectStore`. Global knobs on `cmd/agent` and `cmd/meta`:
`--openbao-addr`, `--openbao-token-path`, `--master-key-name`,
`--encryption-mode`.

Primitives: `storage/internal/crypto/` (Go) and
`storage/dataplane/src/crypto/` (Rust). Transit client:
`storage/internal/openbao/` (Go) and
`storage/dataplane/src/openbao/` (Rust, test-only fake).

See `docs/02-storage-architecture.md` (Encryption section),
`docs/10-identity-and-secrets.md`, and `docs/14-decision-log.md`
(S16/S17/S18, A11/A12) for the authoritative design.

## Components

| Binary                         | Package               | Role                              |
|--------------------------------|-----------------------|-----------------------------------|
| `novanas-storage-controller`   | `cmd/controller/`     | Kubernetes operator               |
| `novanas-storage-agent`        | `cmd/agent/`          | Node DaemonSet agent              |
| `novanas-storage-meta`         | `cmd/meta/`           | Metadata service (chunk-backed)   |
| `novanas-storage-csi`          | `cmd/csi/`            | CSI driver                        |
| `novanas-storage-s3gw`         | `cmd/s3gw/`           | S3 gateway                        |
| `novanas-storage-scheduler`    | `cmd/scheduler/`      | Data-locality scheduler           |
| `novanas-storage-webhook`      | `cmd/webhook/`        | Mutating admission webhook        |
| `novanasctl`                   | `cmd/cli/`            | CLI                               |
| `novanas-dataplane`            | `dataplane/`          | Rust/SPDK data plane              |

## Building

From the repo root (uses the Go workspace):

```sh
go build ./storage/...
go test -short ./storage/...
go vet ./storage/...
```

From this directory:

```sh
make build-all       # all Go binaries
make test            # go test with -race
```

### Rust data plane

The Rust data plane (`dataplane/`) links against SPDK system libraries and is
only built in CI or on a properly configured host. Locally, `cargo check`
from the repo root will recognize the workspace members but a full build
requires SPDK.

See `dataplane/` source for details.

## Go module

Module path: `github.com/azrtydxb/novanas/storage`

## API group

All CRDs under this tree use the API group `novanas.io` (renamed from
NovaStor's `novastor.io`). See `api/v1alpha1/`.

Note: `packages/operators/api/v1alpha1/` (under NovaNas operators) also
declares NovaNas CRDs. Consolidation of the two type sets is a Wave 4 task;
until then the storage-side types are kept in place and continue to be the
source of truth for storage-internal use.

## Contributing

This is now NovaNas code. Changes go through the standard NovaNas PR
workflow. Do not attempt to sync with upstream NovaStor.

## Attribution

Apache 2.0. See the root `NOTICE` file for NovaStor attribution and the
root `LICENSE` for the full license text.
