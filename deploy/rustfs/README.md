# RustFS deployment for NovaNAS

S3-compatible object storage for NovaNAS, running directly on the host as a
systemd service (no k3s, no containers). Backed by a dedicated ZFS dataset
(`tank/objects`) so redundancy, snapshots, and replication are handled by
ZFS while RustFS treats it as a single volume.

## Pinned version

**RustFS `1.0.0-beta.1`** — verified against
<https://github.com/rustfs/rustfs/releases> on 2026-04-29 (most recent tag
at the time of writing). The version is set in `install.sh` via the
`RUSTFS_VERSION` variable.

> **Why pin a beta?** As of 2026-04-29 the upstream project has not yet
> tagged a non-prerelease 1.0. The 1.0.0-beta.x line is the stream the
> upstream installer (`install_rustfs.sh`) consumes. Re-evaluate when
> 1.0.0 GA ships.

## File map (this directory)

| File | Purpose |
| --- | --- |
| `install.sh` | Idempotent installer: downloads pinned binary, creates user/dirs, installs systemd unit, optionally creates ZFS dataset. |
| `rustfs.env.template` | Template for `/etc/rustfs/rustfs.env`. Copied verbatim by `install.sh` only if the target does not exist. |
| `README.md` | This file. |

The systemd unit lives at `deploy/systemd/rustfs.service`, and the Keycloak
client provisioning script lives at `deploy/keycloak/create-rustfs-client.sh`.
End-user documentation (S3 examples, troubleshooting) is in
`docs/objects/README.md`.

## Install

```bash
sudo sh deploy/rustfs/install.sh
```

What it does:

1. Creates `rustfs:rustfs` system user/group.
2. Creates `/var/lib/rustfs` (data + home), `/var/lib/rustfs/data`,
   `/var/log/rustfs`, `/etc/rustfs`, `/etc/rustfs/certs`.
3. Creates ZFS dataset `tank/objects` with `recordsize=1M`, `atime=off`,
   `compression=lz4`, mounted at `/var/lib/rustfs/data` — **only if** the
   `zfs` command is available. Otherwise warns and the operator must
   provision storage manually.
4. Downloads `rustfs-linux-<arch>-musl-v1.0.0-beta.1.zip` from
   GitHub releases, verifies sha256 if a `.sha256` sidecar is published
   (otherwise warns), and atomically installs the binary at
   `/usr/local/bin/rustfs`.
5. Installs `deploy/systemd/rustfs.service` to `/etc/systemd/system/`.
6. Installs `rustfs.env.template` to `/etc/rustfs/rustfs.env` only if the
   target does not already exist (so re-runs preserve operator edits).
7. Runs `systemctl daemon-reload`. **Does not start the service** — the
   operator does that after wiring up TLS and the OIDC client secret.

## TLS

RustFS terminates HTTPS itself. Issue a cert from the Nova CA:

```bash
sudo bash deploy/observability/issue-certs.sh
```

`issue-certs.sh` (in its current form) issues certs for the observability
stack only. Until it is extended to know about RustFS, use the same primitive
locally:

```bash
sudo openssl req -new -newkey rsa:2048 -nodes \
    -keyout /etc/rustfs/certs/rustfs_key.pem \
    -out /tmp/rustfs.csr \
    -subj "/CN=rustfs.novanas.local"
sudo openssl x509 -req -in /tmp/rustfs.csr \
    -CA /etc/nova-ca/ca.crt -CAkey /etc/nova-ca/ca.key -CAcreateserial \
    -out /etc/rustfs/certs/rustfs_cert.pem -days 365 \
    -extfile <(echo "subjectAltName=DNS:rustfs.novanas.local,DNS:novanas.local,IP:127.0.0.1,IP:192.168.10.204")
sudo chown -R rustfs:rustfs /etc/rustfs/certs
sudo chmod 0640 /etc/rustfs/certs/rustfs_*.pem
```

The expected file names (`rustfs_cert.pem`, `rustfs_key.pem` inside
`$RUSTFS_TLS_PATH`) match the upstream RustFS TLS docs.

## OIDC / Keycloak

```bash
sudo bash deploy/keycloak/create-rustfs-client.sh \
    --kc-url https://192.168.10.204:8443 \
    --admin-pass "$KC_ADMIN_PASS" \
    > /tmp/rustfs.json
SECRET=$(jq -r .clientSecret /tmp/rustfs.json)
sudo sed -i \
    "s|^RUSTFS_IDENTITY_OPENID_CLIENT_SECRET=.*|RUSTFS_IDENTITY_OPENID_CLIENT_SECRET=$SECRET|" \
    /etc/rustfs/rustfs.env
```

## Start the service

```bash
sudo systemctl enable --now rustfs.service
journalctl -u rustfs.service -f
```

Service ports:

- **9000** — S3 API (HTTPS)
- **9001** — Console UI (HTTPS, OIDC-protected via Keycloak auth-code).
  The console is served at the path prefix `/rustfs/console`; hitting
  `/` returns the S3 `AccessDenied` XML response because S3 API and
  console share the listener but are routed by URL prefix. Browse to
  `https://<host>:9001/rustfs/console`.

## Upgrade procedure

1. Edit `deploy/rustfs/install.sh` and bump `RUSTFS_VERSION`.
2. Bump the pin in this README.
3. Snapshot the data dataset for rollback safety:
   `sudo zfs snapshot tank/objects@pre-upgrade-$(date +%Y%m%d)`.
4. Re-run `sudo sh deploy/rustfs/install.sh`. The script atomically replaces
   the binary; it does **not** restart the service.
5. `sudo systemctl restart rustfs.service` and watch logs.
6. If the new release fails to start, revert by:
   - putting the previous binary back (you can re-run `install.sh` with
     `RUSTFS_VERSION=<old>` exported), and
   - if on-disk format changed, rolling back the ZFS snapshot.

## Backup story

`tank/objects` is a normal ZFS dataset, so all the standard primitives apply:

```bash
sudo zfs snapshot tank/objects@daily-$(date +%Y%m%d)
sudo zfs send tank/objects@daily-20260429 | ssh backup-host zfs recv pool/rustfs
```

For consistency, RustFS does not require quiescing for snapshots — its
on-disk layout is crash-consistent — but for application-consistent backups
you can `systemctl stop rustfs` briefly around the snapshot.

## Bucket creation runbook

See `docs/objects/README.md` for the full operator runbook (creating the
first bucket, attaching IAM policies to OIDC groups, AWS CLI examples).
