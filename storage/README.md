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

## Components

| Binary                         | Package               | Role                              |
|--------------------------------|-----------------------|-----------------------------------|
| `novanas-storage-controller`   | `cmd/controller/`     | Kubernetes operator               |
| `novanas-storage-agent`        | `cmd/agent/`          | Node DaemonSet agent              |
| `novanas-storage-meta`         | `cmd/meta/`           | Metadata service (Raft)           |
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
