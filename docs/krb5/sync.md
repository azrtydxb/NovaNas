# `nova-krb5-sync`: Keycloak ↔ KDC user-principal sync

`nova-krb5-sync` is a small Go daemon that reconciles users in the
Keycloak `novanas` realm with principals in the embedded MIT KDC. It
runs alongside `nova-api` on the storage host and:

1. Lists every user in Keycloak.
2. Computes the **expected** set of Kerberos principals from each user's
   attributes.
3. Lists the **current** principals in the KDC via the nova-api
   `/krb5/principals` endpoint.
4. Creates anything missing (`randkey=true` — no password) and deletes
   anything that exists in the KDC, matches the user-principal pattern,
   and has no Keycloak counterpart.

It is intentionally idempotent: running it twice in a row produces no
work the second time.

## Principal naming

NovaNAS uses **single-realm tenant-scoped principal names** rather than
cross-realm Kerberos trust. This sidesteps the cross-realm key-exchange
operational hazards while preserving tenant isolation at the
authorization layer (NFS export rules + ZFS ACLs).

| Keycloak user shape                                     | Principal(s) provisioned                            |
|---------------------------------------------------------|-----------------------------------------------------|
| `nova-tenant: acme` (single)                            | `alice/acme@NOVANAS.LOCAL`                          |
| `nova-tenant: acme, foo` (multi-valued)                 | `alice/acme@NOVANAS.LOCAL`, `alice/foo@NOVANAS.LOCAL` |
| no `nova-tenant`, `nova-platform-nfs: true`             | `alice@NOVANAS.LOCAL` (bare-username, no instance)  |
| no `nova-tenant`, no `nova-platform-nfs`                | (nothing — Keycloak-only user)                      |
| `enabled: false`                                        | (nothing — existing principals deleted)             |

The slash in `alice/acme` is Kerberos's standard *instance* delimiter;
this is well-supported across MIT and Heimdal stacks. NFS exports filter
on principal-name patterns (see `internal/host/nfs/`).

### Service principals are never touched

Names matching any of the following prefixes are **excluded** from both
the create-candidate and delete-candidate sets:

- `krbtgt/` (intrinsic to the KDC)
- `kadmin/` (admin interface)
- `nfs/`, `host/` (deployed by `nova-kdc-bootstrap.sh`)
- `K/M` (master key entry)

The full list lives in `internal/krb5sync/sync.go::ServicePrefixes`.

## Configuration

The daemon reads a YAML config (default `/etc/nova-krb5-sync/config.yaml`):

```yaml
novaAPI:
  baseURL: https://nova-api.local:8443
  caCertPath: /etc/nova-ca/ca.crt

oidc:
  issuerURL: https://kc.local:8443/realms/novanas
  clientID: nova-krb5-sync
  clientSecretFile: ${CREDENTIALS_DIRECTORY}/oidc-client-secret
  caCertPath: /etc/nova-ca/ca.crt

keycloak:
  adminURL: https://kc.local:8443
  realm: novanas
  caCertPath: /etc/nova-ca/ca.crt
  insecureSkipVerify: false

krb5:
  realm: NOVANAS.LOCAL

sync:
  stateFile: /var/lib/nova-krb5-sync/state.json
  pollInterval: 5m
  eventInterval: 30s
```

`${CREDENTIALS_DIRECTORY}` is provided by systemd's `LoadCredential=`.
The unit file (`deploy/systemd/nova-krb5-sync.service`) loads the OIDC
client secret from `/etc/nova-krb5-sync/oidc-client-secret` into the
ephemeral credentials tmpfs.

### Flags

- `--config <path>` — YAML config path (default
  `/etc/nova-krb5-sync/config.yaml`).
- `--reconcile-once` — perform a single reconcile and exit. Suitable for
  cron and manual operator invocation.
- `--log-level debug|info|warn|error`.

## Bootstrap

Run `deploy/keycloak/create-krb5-sync-client.sh` with `KC_URL` and
`KC_ADMIN_PASS` set. The script creates (or rotates) the
`nova-krb5-sync` confidential client, grants its service account the
realm role `nova-operator` and the realm-management client roles
`view-users` + `view-events`, and prints
`{"clientId":"...","clientSecret":"..."}` to stdout. Install the secret
under `/etc/nova-krb5-sync/oidc-client-secret` (mode 0640, owned by the
`nova-krb5-sync` system user).

The realm-import file (`deploy/keycloak/realm-novanas.json`) declares
`nova-tenant` and `nova-platform-nfs` as first-class user-profile
attributes so they can be edited in the Keycloak admin UI. `nova-tenant`
is multi-valued and validated against `^[a-z0-9][a-z0-9-]{0,62}$`.

## State file

`/var/lib/nova-krb5-sync/state.json` is the daemon's local state:

```json
{
  "version": 1,
  "lastSyncUnix": 1700000000,
  "lastEventUnix": 1700000050,
  "userPrincipals": {
    "<keycloak-user-uuid>": ["alice/acme@NOVANAS.LOCAL", "alice/foo@NOVANAS.LOCAL"]
  }
}
```

The state file is written atomically (write-temp + rename). Deleting it
is safe: the next reconcile rebuilds it from Keycloak + the KDC's
authoritative principal list. The file exists primarily as a hint for
incremental admin-event processing across restarts and as a debugging
aid.

## Run loop

1. **Phase 1 (initial reconcile, fail-loud)** — perform a full
   reconcile. On Keycloak/KDC unreachable, retry with capped
   exponential backoff (2s, 4s, 8s, ..., 60s + 0–1s jitter). On first
   success, send `READY=1` to systemd via `sd_notify` and proceed to
   phase 2.
2. **Phase 2 (steady state)** — alternate between:
   - A periodic full reconcile every `sync.pollInterval` (default 5m).
   - Admin-event polling every `sync.eventInterval` (default 30s). When
     a `USER` `CREATE`/`UPDATE`/`DELETE` event is observed, trigger an
     immediate full reconcile. Disabled users have their principals
     deleted on the next reconcile.

## Failure semantics

- **Keycloak unreachable** (`ListUsers` fails): the whole reconcile
  pass is aborted (no destructive action without ground truth) and the
  caller backs off.
- **KDC unreachable** (`ListPrincipals` fails): same.
- **Single principal create/delete fails**: log and continue. The next
  reconcile retries the operation. This avoids one bad principal
  poisoning the entire reconcile.

## Troubleshooting

| Symptom                                              | Likely cause / fix                                    |
|------------------------------------------------------|-------------------------------------------------------|
| `403 Forbidden` listing users                        | service account missing `realm-management/view-users` |
| `403 Forbidden` listing admin events                 | service account missing `realm-management/view-events`|
| `403 Forbidden` from nova-api `/krb5/principals`     | service account missing `nova-operator` realm role    |
| Daemon stuck in initial reconcile                    | check `keycloak.service` and `nova-api.service` logs  |
| Created principal then deleted again on next pass    | tenant attribute mistyped or user disabled            |
| Service principals (e.g. `nfs/host`) being deleted   | bug — file an issue, this should never happen         |

## Out of scope (deliberate)

- **Keytab distribution**: the daemon creates principals with random
  keys; it does not export keytabs. End users obtain TGTs via SPNEGO /
  PKINIT or a future `kinit`-style enrollment workflow.
- **Cross-realm trust**: not used. See architectural decision above.
- **Per-tenant KDCs**: not used.
- **Group-based tenant assignment**: only direct user attributes are
  read. Group attributes are not aggregated.
