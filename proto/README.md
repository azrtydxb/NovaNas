# NovaNas protobuf contracts

This directory is the single source of truth for every gRPC / protobuf
contract that crosses a process boundary in NovaNas. It is
[buf](https://buf.build)-managed and lints clean against the STANDARD
rule set (with `PACKAGE_VERSION_SUFFIX` disabled — we use explicit `v1`
directories rather than a package suffix).

## Layout

```
proto/
├── buf.yaml             # module + lint + breaking config
├── buf.gen.yaml         # code generation targets (disabled by default)
├── .gitignore           # excludes gen/
└── novanas/
    ├── common/v1/       # shared scalars: RequestContext, Page, errors
    ├── chunk/v1/        # content-addressed blob API
    ├── metadata/v1/     # volumes, placement (CRUSH), protection policy
    ├── replication/v1/  # snapshot & continuous replication
    ├── dataplane/v1/    # SPDK bdev attach / detach / stats
    └── agent/v1/        # per-node agent: health + local chunk ops
```

## Conventions

- `syntax = "proto3";`
- Package: `novanas.<domain>.v1`
- `option go_package = "github.com/azrtydxb/novanas/proto/gen/go/novanas/<domain>/v1;<domain>v1";`
- One service per `<domain>_service.proto`, domain messages in sibling files
- Request / response types are always named messages (never bare
  primitives), so we can grow fields without breaking wire compat
- Every top-level request embeds a `novanas.common.v1.RequestContext`
  for tracing, tenancy and deadlines
- Streaming for `Watch*` subscriptions and for large-object transfer
- Errors flow through `google.rpc.Status` with an attached
  `novanas.common.v1.ErrorDetail` for structured diagnosis
- Field numbers are sparse (5, 10, 20, ...) in high-churn messages to
  leave room for future additions without renumbering
- `oneof` is used for discriminated unions, notably `ProtectionPolicy`

## Versioning policy

- Every package carries an explicit `vN` directory and package suffix.
- A new incompatible revision lives in a sibling `vN+1/` directory; the
  previous version is preserved for at least one release cycle so that
  consumers can migrate independently.
- `buf breaking` is run in CI against the previous commit; the FILE
  rule set catches wire-breaking renames, renumberings and type
  changes.

## Working with this module

```sh
# Lint every .proto:
buf lint

# Check wire compatibility against main:
buf breaking --against 'https://github.com/azrtydxb/NovaNas.git#branch=main,subdir=proto'

# Regenerate bindings (see buf.gen.yaml for targets):
buf generate
```

Generated code lands under `proto/gen/<lang>/...` and is **gitignored**.
Consumers either (a) regenerate at build time or (b) vendor the
bindings into their own repo. We never commit generated code back into
this directory.

## Relationship to the storage engine

The storage engine ported in Wave 3 (forked NovaStor) has its own,
richer set of internal protobuf contracts — bdev management, raft,
filer inode / dirent CRUD, lock leases, S3 multipart upload state,
shard placement, heal tasks, and more. Those remain internal to the
engine.

The contracts here expose the narrower operator / API surface that
NovaNas drives from the outside. Mappings and shims between the two
live in the operator and API packages, not in this directory.
