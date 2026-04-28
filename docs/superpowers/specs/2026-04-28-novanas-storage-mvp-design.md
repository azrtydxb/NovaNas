# NovaNAS — Storage MVP Design

**Date:** 2026-04-28
**Scope:** v1 storage control plane — ZFS pools, datasets, snapshots, exposed via a Go HTTP API. No shares, apps, VMs, replication, networking config, users, or auth in this milestone.

---

## 1. Goals and non-goals

**Goals**
- A single Go service ("nova-api") that manages ZFS storage on a single host.
- Hybrid source-of-truth: ZFS itself is authoritative for pool/dataset/snapshot existence and properties; Postgres holds metadata ZFS can't store, plus job and audit history.
- Async, durable job model for any operation that shells out to `zfs`/`zpool`.
- Repo and runtime structure that extends cleanly to future host concerns (network, services, sensors) and to user-app management via k3s, without rework.

**Non-goals (this milestone)**
- SMB/NFS/iSCSI shares.
- User identity, authn, authz. Open API on a trusted network.
- Snapshot schedules, replication, send/recv.
- User-installed apps or VMs.
- React UI (the API is what ships; UI follows in a later milestone).
- Multi-node, HA, clustering.
- A/B root host OS (planned later; this design must remain compatible with it).

---

## 2. Architecture

Three host-level services, started by systemd at boot. Baked into the A/B root image when that lands; running on plain Debian-shaped Linux for now.

- **`nova-api.service`** — single Go binary. Runs as root (or with `CAP_SYS_ADMIN`, `CAP_NET_ADMIN` once we narrow it). Listens on inet (admin UI/CLI/remote) and a Unix socket (local CLI).
- **`postgresql.service`** — host package install. Data dir on the boot disk pre-pool; relocatable to a ZFS dataset post-pool if desired (out of scope for v1 — boot disk is fine).
- **`redis.service`** — host package install. Unix socket only; no network listener.

`k3s.service` also runs on the host but **only** as the user-app runtime. nova-api manages it via `client-go` using `/etc/rancher/k3s/k3s.yaml`. K3s is not in the bootstrap path for nova-api, Postgres, or Redis.

```
       host (systemd)
       ├── nova-api.service ──► /sbin/{zfs,zpool,...} (direct exec)
       ├── postgresql.service ◄┐
       ├── redis.service       ├── nova-api unix sockets
       └── k3s.service ──► user app pods (managed via client-go)
```

Bootstrap order is linear: OS up → DB up → Redis up → nova-api up → API operational. K3s health is decoupled from the admin plane: when k3s is broken, the API stays up and can be used to repair it.

---

## 3. API surface

REST, JSON, namespaced under `/api/v1`. Resource representations follow a k8s-shaped envelope (`apiVersion`, `kind`, `metadata`, `spec`, `status`) — no actual k8s involved, but the shape is familiar and leaves room for future fields.

Every state-changing endpoint returns `202 Accepted` with a job id and `Location: /jobs/{id}`. Reads are synchronous.

### Disks (read-only)
- `GET /disks` — list block devices on the host. Source: `lsblk -J` + `/dev/disk/by-id`. Fields: by-id path, size, model, serial, rotational, in-use-by-pool flag.

### Pools
- `GET /pools` — list. Source: `zpool list -H -p`.
- `GET /pools/:name` — detail. Source: `zpool status -P` + `zpool get -H -p all`.
- `POST /pools` — create. Body: vdev layout (`mirror` | `raidz1` | `raidz2` | `raidz3` | `stripe`), disk by-id paths, optional `log` / `cache` / `spare` vdevs. Returns Job.
- `DELETE /pools/:name` — destroy. Returns Job.
- `POST /pools/:name/scrub` — start scrub; `?action=stop` cancels. Returns Job.

### Datasets
- `GET /datasets?pool=:name` — list. Source: `zfs list -H -p -t filesystem,volume`.
- `GET /datasets/:fullname` — detail. Source: `zfs get -H -p all <name>`.
- `POST /datasets` — create. Body: `parent`, `name`, `type` (`filesystem` | `volume`), `properties` map.
- `PATCH /datasets/:fullname` — update properties. Body: `properties` map (only ZFS-known properties; metadata fields go to a separate sub-route, see §5).
- `DELETE /datasets/:fullname` — destroy. Query param `recursive=true` for descendants.

### Snapshots
- `GET /snapshots?dataset=:fullname` — list.
- `POST /snapshots` — create. Body: `dataset`, `name`, `recursive`.
- `DELETE /snapshots/:fullname` — destroy.
- `POST /datasets/:fullname/rollback` — rollback to a snapshot. Body: `snapshot` (short name).

### Jobs
- `GET /jobs/:id` — current state.
- `GET /jobs?state=:state` — list, filterable.
- `GET /jobs/:id/stream` — Server-Sent Events stream of state updates, backed by Redis pub/sub.
- `DELETE /jobs/:id` — cancel (queued: removed from queue; running: SIGTERM the underlying `zfs`/`zpool` process).

### Encoding
`:fullname` for datasets and snapshots is URL-encoded:
- Dataset `tank/home` → `tank%2Fhome`
- Snapshot `tank/home@snap1` → `tank%2Fhome%40snap1`

---

## 4. Host-ops layer

`internal/host/` is the only code that uses `os/exec`. Everything else uses its typed Go API.

```
internal/host/
  exec/        ← shared exec primitive + structured errors
  zfs/         ← pool, dataset, snapshot       (this milestone)
  disks/       ← lsblk, by-id resolution        (this milestone)
  network/     ← bonds, vlans, ip, routes       (placeholder for later)
  services/    ← systemctl wrappers             (placeholder for later)
```

### Exec primitive
```go
// internal/host/exec/exec.go
func Run(ctx context.Context, bin string, args ...string) ([]byte, error)
```
- `exec.CommandContext` with `bin` resolved to an absolute path under `/sbin` or `/usr/sbin`. No shell. Args passed as a slice — no string concatenation, no injection.
- Returns a `*HostError` on non-zero exit carrying exit code and stderr.
- Default 30s deadline via `ctx`. Long-running ops (`zpool create`, `destroy`, `scrub`) skip this and run inside the asynq worker with the worker's context.

### ZFS subpackages
Each module owns its parser and types:
- `internal/host/zfs/pool` — `List`, `Get`, `Create(spec)`, `Destroy`, `Scrub`. Uses `-H -p` (no headers, parsable, exact bytes).
- `internal/host/zfs/dataset` — `List`, `Get`, `Create`, `SetProps`, `Destroy`.
- `internal/host/zfs/snapshot` — `List`, `Create`, `Destroy`, `Rollback`.

Parsers are pure functions `(stdout []byte) → (T, error)`, tested with golden files captured from a real host.

### Validation discipline
Pool, dataset, snapshot, and property names are validated against ZFS naming rules at the **route layer** before reaching `internal/host`. The exec layer trusts its callers.

---

## 5. Data model (Postgres)

Three tables. Migrations via `goose` (`internal/store/migrations/`). Typed query code generated by `sqlc` from `.sql` files in `internal/store/queries/`. Driver is `pgx`.

### `jobs` — durable history
```
id            uuid          pk
kind          text          'pool.create' | 'pool.destroy' | 'pool.scrub'
                            | 'dataset.destroy' | ...
target        text          e.g. 'tank' or 'tank/home'
state         text          'queued'|'running'|'succeeded'|'failed'
                            |'cancelled'|'interrupted'
command       text          full argv joined for display, redacted of secrets
stdout        text
stderr        text
exit_code     int           nullable
error         text          nullable, structured failure reason
created_at    timestamptz
started_at    timestamptz   nullable
finished_at   timestamptz   nullable
request_id    text          for log correlation
```
Asynq's Redis-backed queue is authoritative for live work; Postgres is durable history. On worker startup, any row in `('queued','running')` is marked `'interrupted'` (crash recovery).

### `audit_log` — append-only
```
id            bigserial     pk
ts            timestamptz   default now()
actor         text          nullable in v1 (no auth); 'system' for reconciler
action        text          'pool.create' | 'dataset.update' | ...
target        text
request_id    text
payload       jsonb         redacted request body
result        text          'accepted'|'rejected'
```

### `resource_metadata` — fields ZFS can't store
```
id            bigserial     pk
kind          text          'pool'|'dataset'|'snapshot'
zfs_name      text          natural key
display_name  text          nullable
description   text          nullable
tags          jsonb         {key: value} map, nullable
unique (kind, zfs_name)
```
Rows created on first metadata-setting `PATCH`. Resources without a row are exposed with empty metadata. `DELETE` of a resource also deletes its row (best-effort; orphans are harmless and reaped by a periodic janitor).

A separate sub-route handles metadata to avoid mixing ZFS property updates with non-ZFS metadata in one PATCH:
- `PATCH /pools/:name/metadata`, `PATCH /datasets/:fullname/metadata`, `PATCH /snapshots/:fullname/metadata` accept `{display_name?, description?, tags?}`.

---

## 6. Async ops / job flow

Worker pool runs in the same binary as the HTTP server, started at boot. Library: **asynq** (Redis-backed task queue).

### Flow
1. Handler validates input.
2. Handler writes `audit_log` row + `jobs` row (`state='queued'`) in one Postgres transaction.
3. Handler enqueues an asynq task `{job_id}`. Returns `202` with `Location: /jobs/{id}`.
4. Worker picks up the task:
   - Loads job row, sets `state='running'`, `started_at=now()`.
   - Runs the host-ops call with the worker's context.
   - On finish: writes `stdout`, `stderr`, `exit_code`, sets terminal state, `finished_at=now()`.
   - Publishes a Redis pub/sub event `job:{id}:update` with the new state.
5. `GET /jobs/{id}` reads from Postgres. `GET /jobs/{id}/stream` (SSE) subscribes to the Redis channel.

### Concurrency rules
- **Per pool**: at most one running job. Asynq `unique` constraint keyed by pool name. Prevents scrub-during-destroy and overlapping scrubs.
- **Per dataset**: at most one running destructive op; reads unrestricted.
- **Pool-create**: cluster-wide concurrency 1. Single-node, kernel disk pickup is racy under load.

### Cancellation
- `DELETE /jobs/{id}` on a `queued` job removes the asynq task.
- On a `running` job, the worker's context is cancelled, which kills the underlying `zfs`/`zpool` process via `cmd.Cancel`. Some ops (a partway pool create) won't cancel cleanly — we mark the job `cancelled` and let any partial state surface on the next live read.

### Crash recovery
On worker startup:
```sql
UPDATE jobs SET state='interrupted', error='process restarted'
 WHERE state IN ('running','queued');
```
Asynq re-delivers in-flight tasks per its retry config. The worker checks the row state before re-running: only re-runs if `state IN ('queued','interrupted')` after re-load. This makes job execution idempotent against crash-redelivery.

---

## 7. Project layout

Single Go module at the repo root.

```
nova-nas/
├── cmd/
│   └── nova-api/main.go                 ← entrypoint; flags, config, wires HTTP+worker
├── internal/
│   ├── api/                             ← HTTP layer
│   │   ├── server.go                    ← chi router, middleware
│   │   ├── middleware/                  ← request id, logging, recover, json errors
│   │   ├── handlers/
│   │   │   ├── disks.go
│   │   │   ├── pools.go
│   │   │   ├── datasets.go
│   │   │   ├── snapshots.go
│   │   │   └── jobs.go
│   │   └── oapi/                        ← oapi-codegen output (committed)
│   ├── host/
│   │   ├── exec/                        ← shared exec primitive + structured errors
│   │   ├── zfs/                         ← pool, dataset, snapshot
│   │   └── disks/                       ← lsblk, by-id
│   ├── runtime/                         ← k3s client (placeholder for later)
│   │   └── client/
│   ├── jobs/                            ← asynq worker, task definitions, dispatcher
│   ├── store/                           ← sqlc-generated query code + migrations
│   │   ├── queries/*.sql
│   │   ├── migrations/*.sql             ← goose
│   │   └── gen/                         ← sqlc output
│   └── config/                          ← env loading, validation
├── api/
│   └── openapi.yaml                     ← hand-authored; SoT for handlers + TS client
├── deploy/
│   ├── systemd/
│   │   ├── nova-api.service
│   │   ├── nova-api.socket              ← optional socket-activated admin unix socket
│   │   └── tmpfiles.d/nova-api.conf
│   ├── postgres/
│   │   └── nova-api-init.sql            ← createdb + role
│   └── packaging/
│       └── debian/                      ← .deb packaging (later, for A/B image)
├── test/
│   ├── fixtures/                        ← captured zfs/zpool output for parser tests
│   ├── integration/                     ← DB+worker, no real ZFS
│   └── e2e/                             ← real ZFS, sparse loopback devices
├── scripts/
│   ├── gen-openapi.sh                   ← runs oapi-codegen
│   └── gen-sqlc.sh                      ← runs sqlc
├── go.mod
└── README.md
```

### Build & deploy
- Single static binary, CGO disabled.
- Installed under `/usr/bin/nova-api` by the `.deb` (later) or copied directly during dev.
- systemd unit shipped under `deploy/systemd/`.

### Config
Env-only, parsed via `envconfig`. Required:
- `DATABASE_URL`
- `REDIS_URL` (defaults to `unix:///run/redis/redis.sock`)
- `LISTEN_ADDR`
- `ZFS_BIN` (default `/sbin/zfs`)
- `ZPOOL_BIN` (default `/sbin/zpool`)
- `LOG_LEVEL`

---

## 8. Testing

### Unit (`*_test.go` next to code)
- Parsers: golden-file tests against captured `zfs`/`zpool` output in `test/fixtures/`.
- Validators: table-driven, cover every accepted/rejected name shape per ZFS rules.
- Job state transitions: pure-function tests, no DB.

### Integration (`test/integration`, `-tags=integration`)
- `testcontainers-go` for Postgres + Redis.
- Stub the host-exec layer with a fake that returns prerecorded stdout/stderr/exit codes.
- Drive HTTP → audit → enqueue → worker → DB → SSE end-to-end. Asserts on rows, asynq state, SSE events.

### End-to-end (`test/e2e`, `-tags=e2e`)
- Runs only on a host with ZFS available.
- Provisions sparse files via `truncate` + `losetup` for fake disks.
- Creates pools, datasets, snapshots; asserts API + ZFS state agree.
- Each test uses a unique pool name; tear down on completion.

### CI
- GitHub Actions: lint (`golangci-lint`), unit, integration on every PR.
- E2E on a self-hosted runner (VM with ZFS) on PRs labeled `needs-e2e` and on every merge to main.

### Mocking discipline
No mocks inside `internal/host`. Those packages own real exec; their tests are golden-file (unit) or e2e (real ZFS). Mocking happens at the **consumer** boundary — handlers and worker — by injecting an interface that `internal/host/zfs` (etc.) satisfies.

---

## 9. Security and isolation

- nova-api binds to a configured admin interface; not all interfaces by default. Host firewall (nftables) managed by nova-api in a later milestone restricts inbound to admin networks.
- Postgres and Redis listen on Unix sockets only.
- All `os/exec` calls use absolute binary paths and arg slices — no shell, no concatenation.
- ZFS naming and property validation happens at the route layer before exec.
- User apps (later) run in k3s under `nova-apps-*` namespaces with PSA `restricted`, default-deny NetworkPolicy, and ResourceQuota/LimitRange. They have no path to the API except via the host IP, which the host firewall blocks from the cluster CIDR.
- Audit log captures every state-changing call; secrets are redacted from `payload` before write.

---

## 10. Future-compatibility notes

These constrain the v1 design even though they're out of scope to implement now.

- **A/B root host OS**: system services are baked into the OS image and upgrade atomically with a partition swap. Avoid runtime state that lives outside Postgres or known on-disk locations. Keep config in env + small files.
- **User apps in k3s**: `internal/runtime/` exists as a placeholder so the import structure and SA/RBAC plumbing can land before app endpoints do. App definitions will live in Postgres (desired state) with k3s as the executor; reconciliation triggered on write plus periodic resync.
- **Network/services/sensors as host-ops**: same `internal/host/<domain>/` shape as `zfs`. Adding domains adds packages, not architecture.
- **Authn/authz**: the `audit_log.actor` column is nullable today and the API is open. When auth lands, the column gets populated and a middleware enforces it; no schema change to the rest of the model.
- **React UI**: served by a host nginx (or the API binary itself) as static files. The API exposes an OpenAPI spec that drives a typed TS client.
