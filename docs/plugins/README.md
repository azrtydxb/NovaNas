# NovaNAS Tier 2 Plugin Engine — Operator Guide

Tier 2 plugins are NovaNAS-published, cosign-signed packages that
extend nova-api with a marketplace-installable surface: each plugin
adds an Aurora UI window and may mount its own reverse-proxied API
under `/api/v1/plugins/{name}/api/*`.

This guide covers the lifecycle from an operator perspective. For
plugin authors see `authoring.md`.

## Tiers at a glance

| Tier | What | Where it lives |
|------|------|----------------|
| 1 — Core | Bundled with the OS image (Samba, NFS, ZFS tools). Atomic with the image. | `internal/host/*` |
| 2 — Plugins | NovaNAS-signed, marketplace-installable. Extend the API + Aurora UI. | `internal/plugins/*` (this engine) |
| 3 — Community apps | Helm charts on the embedded k3s. No API/UI integration. | `internal/workloads/*` (separate, settled) |

## Configuration

Environment variables read by `nova-api`:

| Var | Default | Purpose |
|-----|---------|---------|
| `MARKETPLACE_INDEX_URL` | `https://raw.githubusercontent.com/azrtydxb/NovaNas-packages/main/index.json` | The catalog endpoint. |
| `MARKETPLACE_TRUST_KEY_PATH` | `/etc/nova-nas/trust/marketplace.pub` | Cosign public key used to verify package signatures. |
| `MARKETPLACE_COSIGN_BIN` | (unset) | When set, the verifier shells out to `cosign verify-blob` instead of using the native-Go path. |
| `PLUGINS_ROOT` | `/var/lib/nova-nas/plugins` | On-disk root for unpacked plugin trees. |

The trust key file is shipped with the OS image. Operators who want
full sigstore semantics (rekor, transparency) can install the cosign
binary and set `MARKETPLACE_COSIGN_BIN=/usr/bin/cosign`.

## Permissions

| Role | Read catalog + installed list | Install / upgrade / uninstall |
|------|-------------------------------|-------------------------------|
| `nova-admin` | yes | yes |
| `nova-operator` | yes | no |
| `nova-viewer` | yes | no |

The `nova:plugins:read` permission gates GET endpoints; `nova:plugins:admin`
gates state changes. `nova:plugins:write` exists for future fine-grained
controls but is currently equivalent to admin.

## Browse the catalog

```sh
curl -H "Authorization: Bearer $TOK" https://nas.local/api/v1/plugins/index
curl -H "Authorization: Bearer $TOK" https://nas.local/api/v1/plugins/index?refresh=true
curl -H "Authorization: Bearer $TOK" https://nas.local/api/v1/plugins/index/rustfs
```

The catalog is cached server-side for 15 minutes; `?refresh=true`
forces a fresh fetch from the marketplace.

## Install

```sh
curl -X POST -H "Authorization: Bearer $TOK" \
     -H "Content-Type: application/json" \
     -d '{"name":"rustfs","version":"1.2.3"}' \
     https://nas.local/api/v1/plugins
```

What happens behind the scenes:

1. Marketplace lookup — `(name, version)` resolved against the cached
   index.
2. Tarball + signature downloaded.
3. Cosign verification against the operator-supplied trust key.
4. Tarball extracted to `$PLUGINS_ROOT/<name>/`.
5. Manifest validated (apiVersion, name, version, category gates).
6. `needs:` block fulfilled in order — datasets, oidcClient, tlsCert,
   permission. Failures roll back already-completed steps.
7. Helm release / systemd unit deployed.
8. API routes mounted under `/api/v1/plugins/<name>/api/*`.
9. UI bundle registered for serving at `/api/v1/plugins/<name>/ui/*`.
10. Row written to the `plugins` table; `plugin_resources` rows
    record the auto-provisioned IDs for cleanup.

## Upgrade

```sh
curl -X PATCH -H "Authorization: Bearer $TOK" \
     -H "Content-Type: application/json" \
     -d '{"version":"1.2.4"}' \
     https://nas.local/api/v1/plugins/rustfs
```

Upgrades are atomic at the routing layer: the new version's API
routes and UI bundle replace the old in-process before the previous
runtime is shut down. Auto-provisioned `needs:` resources are
preserved across upgrades — the new version sees the same dataset,
oidcClient, and tlsCert.

## Uninstall

```sh
# Remove the plugin but keep its dataset/oidcClient/tlsCert.
curl -X DELETE -H "Authorization: Bearer $TOK" \
     https://nas.local/api/v1/plugins/rustfs

# Remove everything, including auto-provisioned resources.
curl -X DELETE -H "Authorization: Bearer $TOK" \
     "https://nas.local/api/v1/plugins/rustfs?purge=true"
```

`?purge=true` is destructive: a `dataset` need will issue a
`zfs destroy`, `oidcClient` deletes the Keycloak client, and
`tlsCert` revokes the cert. Without purge those resources stay
behind so a re-install picks them up unchanged.

## Restart-at-boot

On `nova-api` startup the engine reads the `plugins` table and:

- Re-mounts every plugin's API routes.
- Re-registers every plugin's UI bundle for serving.

Plugin processes (helm releases / systemd units) start on their own
substrate; nova-api just re-attaches the proxy handlers.

## Troubleshooting

### "marketplace_unreachable"

The configured `MARKETPLACE_INDEX_URL` is not returning 200. Check
network access and that the URL is reachable from the host. Try:

```sh
curl -v "$MARKETPLACE_INDEX_URL"
```

### "signature: ecdsa verify failed"

The downloaded tarball does not match the signature against the
trust key. Either the package was tampered with, the trust key is
stale, or the marketplace served a mismatched signature. Re-pull the
trust key:

```sh
curl -fsSL https://raw.githubusercontent.com/azrtydxb/NovaNas-packages/main/trust/novanas-marketplace.pub \
    | sudo tee /etc/nova-nas/trust/marketplace.pub
```

Then retry the install.

### Install hangs at "needs:" step

Auto-provisioned resources may take several seconds (Keycloak admin
creates can be slow on a busy realm). The default timeout is 30s per
provisioner step. Check `journalctl -u nova-api` for the specific
step that is blocked.

### Plugin UI window does not appear in Aurora

Aurora reads the manifest's `spec.ui.window` block from
`/api/v1/plugins/<name>` and dynamically imports
`/api/v1/plugins/<name>/ui/main.js`. If the window is missing:

1. Confirm the plugin shows up in `GET /api/v1/plugins`.
2. Confirm `curl /api/v1/plugins/<name>/ui/main.js` returns 200.
3. Reload Aurora — the chrome reads installed plugins on dashboard
   mount.

### Reverse-proxied API calls return 503

`service-token` auth mode requires a `ServiceTokenMinter` to be
wired in `nova-api`. If your install only uses `bearer-passthrough`
plugins this should not trigger. Check the manifest's
`spec.api.routes[].auth` and switch to `bearer-passthrough` if your
upstream verifies the caller's JWT directly.

### Database migration

The `plugins` and `plugin_resources` tables ship in migration
`0006_plugins.sql`. Apply with:

```sh
goose -dir internal/store/migrations postgres "$DATABASE_URL" up
```

## Related

- `docs/plugins/authoring.md` — guide for plugin developers.
- The marketplace repo (separate): https://github.com/azrtydxb/NovaNas-packages
- Tier 3 community apps: `/api/v1/workloads/*`
