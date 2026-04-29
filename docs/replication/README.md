# NovaNAS Replication

NovaNAS's replication subsystem moves data between a NovaNAS host and an
arbitrary destination on a schedule. Three backends ship in v1:

| Backend | Use case                          | Fidelity                    |
| ------- | --------------------------------- | --------------------------- |
| `zfs`   | NovaNAS-to-NovaNAS                 | snapshots + props + ACLs    |
| `s3`    | Cloud DR / RustFS / MinIO          | files only                  |
| `rsync` | Legacy non-NovaNAS over SSH        | POSIX ACLs/xattrs           |

A replication is described by a `Job`:

| Field         | Notes                                                         |
| ------------- | ------------------------------------------------------------- |
| `backend`     | `zfs` \| `s3` \| `rsync`                                      |
| `direction`   | `push` (local source → remote dst) \| `pull` (remote → local) |
| `source`      | per-backend (dataset, path, bucket+prefix, host+sshUser)      |
| `destination` | per-backend (same shape as source)                            |
| `schedule`    | standard cron expression; empty = manual-only                 |
| `retention`   | `keepLastN`, `keepDaily`, `keepWeekly`, `keepMonthly`, `keepYearly` |
| `secretRef`   | OpenBao key prefix for credentials, e.g. `nova/replication/<id>` |

## Backends

### ZFS native

Runs `zfs send | zfs receive` over an SSH command channel. The first
run is a full send; every subsequent run uses `-i <last_snapshot>` for
an incremental delta. The "last snapshot" pointer is stored on the Job
row and updated automatically when a run succeeds.

A per-job snapshot is created at the start of the run with short name
`repl-YYYY-MM-DD-HHMM`. Retention applies to those snapshots.

Required fields:

- `direction=push`: `source.dataset`, `destination.host`, `destination.dataset`
- `direction=pull`: `source.host`, `source.dataset`, `destination.dataset`

The SSH key for the remote zfs receive is fetched from
`nova/replication/<job-id>/ssh_key` in OpenBao.

### S3 (push or pull)

Wraps any S3-compatible API (RustFS, MinIO, AWS S3, Wasabi, ...). Push
walks the local source path and `PUT`s objects under the bucket
prefix. Pull `LIST`s the bucket prefix and writes each object to the
local destination.

Required fields:

- `direction=push`: `source.path`, `destination.bucket` (and optional
  `destination.prefix`, `destination.endpoint`, `destination.region`)
- `direction=pull`: `source.bucket` (+ optional `source.prefix`/
  `source.endpoint`/`source.region`), `destination.path`

S3 credentials come from `nova/replication/<job-id>/access_key` and
`secret_key`. SSE-S3 / SSE-KMS settings, bucket versioning and
lifecycle rules are configured on the bucket itself; the backend
deliberately stays out of the way.

> **Implementation note:** the S3 backend uses a narrow `S3Client`
> interface internally so the package compiles without the AWS SDK.
> A production deployment wires an `aws-sdk-go-v2`-backed
> implementation in `cmd/nova-api/main.go`.

### rsync over SSH

Shells out to `/usr/bin/rsync -aAXH --delete --stats`. Idempotent by
construction. Lower fidelity than ZFS — POSIX ACLs and xattrs are
preserved but ZFS-specific snapshot history is not.

Required fields:

- `direction=push`: `source.path`, `destination.host`,
  `destination.sshUser`, `destination.path`
- `direction=pull`: `source.host`, `source.sshUser`, `source.path`,
  `destination.path`

The SSH key is fetched from `nova/replication/<job-id>/ssh_key` and
written to a temporary file passed to rsync via `-e "ssh -i <file>"`.

## Scheduling

The `schedule` field is a standard 5-field cron expression. The
existing scheduler (`internal/host/scheduler`) reads enabled jobs each
tick, evaluates `ShouldFireBetween(prev, now)` for each, and dispatches
an Asynq task on each match. Jobs with an empty `schedule` only run
when `/api/v1/replication-jobs/{id}/run` is called.

## Retention

Retention is applied after every successful run:

- `keepLastN`: keep the N most recent successful runs.
- `keepDaily` / `keepWeekly` / `keepMonthly` / `keepYearly`: sanoid-style
  calendar buckets — keep the newest entry per day/week/month/year up
  to the configured count.

Setting all retention fields to zero disables pruning entirely.

## API

All endpoints live under `/api/v1/replication-jobs`. Read endpoints
require the `nova:replication:read` permission (granted to
`nova-viewer` and above); writes and ad-hoc triggers require
`nova:replication:write` (granted to `nova-operator` and above).

| Method | Path                                  | Description                       |
| ------ | ------------------------------------- | --------------------------------- |
| POST   | `/api/v1/replication-jobs`            | create                            |
| GET    | `/api/v1/replication-jobs`            | list                              |
| GET    | `/api/v1/replication-jobs/{id}`       | detail (with last N runs)         |
| PATCH  | `/api/v1/replication-jobs/{id}`       | partial update                    |
| DELETE | `/api/v1/replication-jobs/{id}`       | delete                            |
| POST   | `/api/v1/replication-jobs/{id}/run`   | trigger one-shot run              |
| GET    | `/api/v1/replication-jobs/{id}/runs`  | run history                       |

### Examples

```bash
# Create a nightly ZFS push at 02:30 keeping the last 14 successful runs
curl -X POST -H 'Content-Type: application/json' \
  -d '{
    "name": "nightly-data",
    "backend": "zfs",
    "direction": "push",
    "source": {"dataset": "tank/data"},
    "destination": {"dataset": "backup/data", "host": "nas2.local", "sshUser": "nova"},
    "schedule": "30 2 * * *",
    "retention": {"keepLastN": 14}
  }' \
  https://nova.local/api/v1/replication-jobs

# List all jobs
curl https://nova.local/api/v1/replication-jobs

# Trigger an ad-hoc run
curl -X POST https://nova.local/api/v1/replication-jobs/${ID}/run

# Run history
curl https://nova.local/api/v1/replication-jobs/${ID}/runs

# Update the schedule
curl -X PATCH -H 'Content-Type: application/json' \
  -d '{"schedule": "*/15 * * * *"}' \
  https://nova.local/api/v1/replication-jobs/${ID}

# Delete
curl -X DELETE https://nova.local/api/v1/replication-jobs/${ID}
```

## Troubleshooting

- A run stuck in `running` past its lock TTL (default 6 hours) means
  either the worker died mid-run or the destination is unreachable.
  Check the audit log for entries tagged `replication.run`. The lock
  is force-released after TTL.
- `ErrLocked` from `/run` means another run is already in flight for
  the same job. This is expected and not an error condition; retry.
- ZFS incremental sends fail if the previous snapshot was destroyed on
  the destination. Clear `lastSnapshot` on the Job (PATCH it to
  `""`) to force a full send on the next run.
- S3 PUTs failing with `SignatureDoesNotMatch` usually mean the bucket
  region does not match `destination.region`. Endpoint URLs for
  RustFS/MinIO must include the scheme (`https://`).
- rsync exits with code 23 for "partial transfer due to error" — the
  most common cause is destination filesystem permission errors.
