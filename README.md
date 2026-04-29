# NovaNAS

Storage control plane for a single-node ZFS-based NAS appliance. A Go HTTP API on host systemd, backed by Postgres and Redis, that manages ZFS pools, datasets, and snapshots, plus the async job system that runs them.

## Architecture

- `nova-api.service` — single Go binary, runs on the host (or in `gcr.io/distroless` for CI/dev)
- `postgresql.service` — durable state: `jobs`, `audit_log`, `resource_metadata`
- `redis.service` — asynq job queue + SSE pub/sub
- ZFS lives on the host; the binary shells out to `/sbin/{zfs,zpool}` via the `internal/host/exec` primitive

The OpenAPI 3.1 spec at `api/openapi.yaml` is the source of truth for the API. Go types under `internal/api/oapi/` and the future TS client under `clients/typescript/src/` are both generated from it.

## Build & test

```
make build          # static binary -> bin/nova-api
make test           # unit
make test-integration  # testcontainers Postgres+Redis (requires Docker)
make test-e2e       # real ZFS via sparse loopback (requires root + zfs)
make gen            # regenerate sqlc + oapi types
```

## Run locally (dev)

Requires Postgres + Redis on the host, and the schema applied:

```
make migrate-up DB_URL=postgres://novanas:novanas@127.0.0.1:5432/novanas?sslmode=disable

DATABASE_URL=postgres://novanas:novanas@127.0.0.1:5432/novanas?sslmode=disable \
REDIS_URL=redis://127.0.0.1:6379/0 \
LISTEN_ADDR=:8080 \
LOG_LEVEL=info \
./bin/nova-api
```

## Production install (Debian)

Build a `.deb` on a host with `dpkg-buildpackage`:

```
dpkg-buildpackage -us -uc -b
```

The package installs `/usr/bin/nova-api`, the systemd unit, tmpfiles entries, and writes a sensible default `/etc/nova-api/env`. Postinst enables and starts the service.

Two additional oneshot units handle target-config persistence across reboots:

- `nova-nvmet-restore.service` — replays the saved NVMe-oF configfs tree from `/etc/nova-nas/nvmet-config.json` after `sys-kernel-config.mount` and before `nova-api.service`.
- `nova-iscsi-restore.service` — invokes `targetctl restore` to reapply the saved LIO/iSCSI target. On Debian this is effectively a redundant safety net for `targetclid.service`; on distros without `targetclid` it is the only thing that brings iSCSI back after reboot.

Enable both alongside `nova-api`:

```
sudo systemctl enable nova-nvmet-restore.service nova-iscsi-restore.service
```

Bootstrap the database:

```
sudo -u postgres psql -f /usr/share/nova-api/nova-api-init.sql
```

## E2E runner

The `e2e` workflow targets a self-hosted runner labeled `[self-hosted, zfs]`. Provision a Linux VM with:

- Ubuntu 22.04+ or Debian 12+
- `zfsutils-linux` installed and `/dev/zfs` accessible
- Runner user has passwordless sudo (for `losetup` and ZFS ops)
- GitHub Actions runner registered with the labels above

## Documentation

- `docs/superpowers/specs/` — design docs
- `docs/superpowers/plans/` — implementation plans
- `api/openapi.yaml` — API contract
