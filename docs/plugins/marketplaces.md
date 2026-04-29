# Marketplaces — operator runbook

The Tier 2 plugin engine talks to a *registry* of marketplaces, not a single hardcoded source. Each marketplace publishes an `index.json` plus signed tarballs and pins its own cosign public key. Operators add additional marketplaces (TrueCharts, third-party publishers, internal mirrors); the locked **novanas-official** entry is seeded at boot from `MARKETPLACE_INDEX_URL` + `MARKETPLACE_TRUST_KEY_PATH` and cannot be deleted or disabled.

## Trust model

* `trust_key_pem` — the **source of truth** at install time. Every signature verification uses the PEM stored on the row, never a freshly-fetched key.
* `trust_key_url` — a *hint* used only when the operator explicitly calls `POST /marketplaces/{id}/refresh-trust-key`. We never auto-refresh; otherwise a compromised marketplace could rotate its key after gaining trust.
* The locked novanas-official entry has no `trust_key_url` set by default — its key is the file at `MARKETPLACE_TRUST_KEY_PATH` and is refreshed by replacing that file and restarting nova-api.

## Adding a marketplace (TrueCharts example)

```bash
curl -sSf -X POST https://nova-api.local/api/v1/marketplaces \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "truecharts",
    "indexUrl": "https://charts.truecharts.example/index.json",
    "trustKeyUrl": "https://charts.truecharts.example/cosign.pub"
  }'
```

The server validates that:

1. `indexUrl` is reachable (HTTP `HEAD`; `405 Method Not Allowed` is accepted).
2. `trustKeyUrl` returns a parseable PEM-encoded ECDSA / RSA / Ed25519 public key.

If either check fails the row is *not* persisted. On success the response includes the pinned `trustKeyPem` so you can audit it. RBAC: `nova:marketplaces:admin`.

## Disabling vs deleting

* `PATCH /marketplaces/{id}` with `{"enabled": false}` removes the marketplace from `FetchAll` (it is not searched for installs/upgrades) but preserves the row, the pinned key, and any plugins installed from it. Operators use this when they want a reversible "pause".
* `DELETE /marketplaces/{id}` removes the row outright. It does *not* uninstall plugins that were installed from that marketplace; existing installations remain trusted.
* Both calls return **409 Conflict** on the locked novanas-official entry.

## Refreshing a marketplace's trust key

When the upstream rotates its cosign key, run:

```bash
curl -sSf -X POST \
  https://nova-api.local/api/v1/marketplaces/$ID/refresh-trust-key \
  -H "Authorization: Bearer $TOKEN"
```

The server hits `trust_key_url`, fetches the new PEM, validates it parses as a public key, and atomically pins it. Every refresh is recorded in the audit log. Until a refresh is run, *new* installs continue to be verified against the previously pinned key — you decide when to accept the rotation.

## Naming collisions across marketplaces

When two marketplaces publish a plugin with the same name (`rustfs` is in both novanas-official and `extra`), the merged catalog returns **both** entries, each tagged with its source marketplace. Installing without disambiguation prefers the locked entry first, then registration order. To pin to a specific source, pass `marketplaceId` in the install request:

```bash
curl -sSf -X POST https://nova-api.local/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{"name":"rustfs","version":"1.2.3","marketplaceId":"<uuid>"}'
```

The audit row captures which marketplace the install was sourced from, so you can later answer "which plugins came from TrueCharts?".

## Auditing what came from where

The merged index returned by the engine carries a `sources` block: `{ id, name, indexUrl, pluginCount, status, fetchedAt }` per marketplace. Combine it with the per-installation audit log (`InsertAudit` rows tagged `plugin.install`) to build the report:

```sql
SELECT a.target AS plugin, a.payload->>'marketplaceId' AS source
FROM audit_log a
WHERE a.action = 'plugin.install'
ORDER BY a.ts DESC;
```

## Revoking a marketplace

To stop trusting a marketplace immediately:

1. `PATCH /marketplaces/{id}` with `{"enabled": false}` — disables installs/upgrades from this source.
2. *Optional but recommended:* `DELETE /api/v1/plugins/{name}?purge=true` for every plugin installed from it. Existing on-disk binaries are NOT auto-removed when you delete or disable a marketplace.
3. `DELETE /marketplaces/{id}` removes the registry row.

The locked novanas-official entry cannot be revoked; on a tightly-locked appliance, replace `MARKETPLACE_INDEX_URL` and `MARKETPLACE_TRUST_KEY_PATH` at install time and the seed will use those instead.

## Failure modes

* Index unreachable at fetch time — `FetchAll` continues with the remaining sources and reports `status: "error"` on the failing one. Plugins from healthy sources still appear in the merged catalog.
* Invalid trust key on refresh — the row is left untouched and the call returns 502.
* `enabled=false` on a locked entry — 409.
* Duplicate `name` — 409.
* Reserved name `novanas-official` on POST — 409.
