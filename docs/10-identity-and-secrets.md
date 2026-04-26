# 10 — Identity & Secrets

Two upstream components own these responsibilities:

- **Keycloak** — all identity and access management
- **OpenBao** — all secrets, PKI, encryption-as-a-service

The NovaNas UI hides both behind domain-shaped screens; admins only interact with Keycloak/OpenBao consoles via escape-hatch paths.

## Keycloak

### Role

- User accounts (local) — passwords hashed, TOTP, WebAuthn, recovery codes
- Groups + roles
- Sessions, token refresh, logout-everywhere
- OIDC provider for the NovaNas UI (code + PKCE)
- Federation: AD, LDAP, SAML, social IdPs, external OIDC — all as Keycloak adapters, zero custom code per provider
- Password policies, account lockout, brute-force detection
- Admin API for programmatic user/group CRUD

### Deployment

- Upstream Keycloak 26+ (LTS with declarative config)
- Deployed in `novanas-system` via Helm subchart
- Postgres backend (shared instance, separate schema)
- Realm **`novanas`** pre-provisioned on first boot
- DB credentials and realm admin secrets loaded from OpenBao via Vault Agent init container

### Single realm, groups for scope

- One realm (`novanas`) contains all users, groups, roles
- Groups represent access scopes (`admins`, `users`, `family`, custom groups for shares)
- Roles control permission (`admin`, `user`, `viewer`, `share-only`)
- No per-tenant realms — simpler, scales to thousands of users

### NovaNas ↔ Keycloak integration

- `user` and `group` API resources are **projections** of Keycloak users/groups, synced by the `keycloak-sync` controller into Postgres
- Source of truth for authentication = Keycloak; the projected `user` / `group` API resources exist so audit, authorization scopes, and UI can reference them by stable IDs
- `keycloakRealm` / `keycloakFederationProvider` API resources wrap Keycloak realm config (federation providers, custom claims, etc.)
- The NovaNas API server validates incoming JWTs, extracts claims (sub, groups, roles), and resolves NovaNas permissions from them

### Theme

Keycloak login page uses a custom NovaNas theme — admin sees branded login, not default Keycloak. Theme is a ConfigMap applied at Keycloak startup.

### Escape hatch

Raw Keycloak admin console is accessible at `https://nas.local/auth/admin/` for power users who need realm-level tweaks. Admin console login uses the same NovaNas admin credentials via the realm's admin role. Use is audited.

## OpenBao

### Role

- **Secrets store** for everything sensitive:
  - Chunk engine master key (MK)
  - Per-volume DKs (wrapped by MK)
  - `kmsKey` API resource backing material
  - TLS private keys (internal PKI + ACME-issued)
  - Keycloak DB password
  - Postgres passwords
  - Replication auth tokens
  - Cloud backup credentials
  - API tokens, SSH keys
- **PKI engine** — internal CA issuing service-to-service mTLS certs with automatic rotation
- **Transit engine** — encrypt/decrypt without surfacing keys: chunk engine calls `transit/encrypt/<key>` and `transit/decrypt/<key>`, master key stays in OpenBao
- **Dynamic secrets** — scoped Postgres credentials per workload, rotating app creds on opt-in

### Deployment

- Upstream OpenBao (Vault fork) packaged into the appliance image
- Postgres storage backend (shared instance, separate schema)
- Runs in the system tenant; the runtime adapter places it as a Pod (K8s) or container (Docker)

### Unseal

- **TPM auto-unseal** — Shamir-split unseal key is sealed to the box's TPM, bound to measured-boot PCRs
- Booting a different OS on the same hardware fails to unseal (measurement mismatch)
- Admin-provided **recovery passphrase** is the DR path: unseal keys are exported encrypted with this passphrase and included in the `configBackupPolicy` output
- No operator types unseal keys during normal boots

### Path-scoped ACLs

Policies use path prefixes matching NovaNas resource shapes:

```
novanas/buckets/{name}/dk
novanas/datasets/{namespace}/{name}/dk
novanas/apps/{namespace}/{app}/*
novanas/pki/*
novanas/replication/{target}/token
novanas/kms/keys/{name}
novanas/chunk-engine/master-key
novanas/backup/*
```

Each workload authenticates with the workload identity provided by the runtime adapter (OpenBao's `kubernetes` auth method on K8s; an OpenBao JWT auth method bound to the API server's signing key on Docker) and receives a scoped token matching its workload identity.

### Chunk engine integration

- Chunk engine workload identity binds to OpenBao via the runtime-appropriate auth method
- On mount: calls `transit/decrypt/master-key/{bucket-dk-wrapped}` → receives unwrapped DK
- DK cached in-process for the life of the mount
- Chunk encryption uses convergent-derived per-chunk key (topic 02); DK never persists outside OpenBao
- Key rotation: OpenBao `transit/keys/master-key/rotate` → all wrapped DKs re-wrapped server-side, chunks remain encrypted with old DK (content-addressed under it), new writes use rotated derivation

### Escape hatch

- OpenBao CLI available from admin shell
- `bao` binary present on host OS
- Root token recoverable via recovery passphrase + unseal keys ceremony (documented but strongly discouraged for routine use)
- All root-token usage audited

## Secret reference pattern

API resources that need secrets use explicit OpenBao paths, not runtime Secret/Config names:

```json
{
  "auth": { "secretRef": "openbao://novanas/replication/offsite-token" }
}
```

The **OpenBao Agent** (sidecar pattern, standard in Vault/OpenBao) translates these at workload admission into mounted secret files inside the container. Secrets are never persisted as Kubernetes Secrets or Docker config — those runtimes only see the mount, not the value.

Exception: bootstrap secrets (OpenBao's own seal context, initial Postgres password before OpenBao is up) live as runtime-native secrets briefly. These are minimal and rotated once the system is up.

## certificate

Delegates to OpenBao PKI and/or novaedge's ACME client.

`POST /api/v1/certificates`:

```json
{
  "name": "plex-cert",
  "dnsNames": ["plex.nas.local"],
  "issuer": {
    "type": "internal-pki",
    "pkiPath": "novanas/pki/services"
  },
  "duration": "90d",
  "renewBefore": "30d"
}
```

Status fields populated by the certificate controller include `secretRef: openbao://novanas/certs/plex-cert`, `notBefore`, `notAfter`. novaedge reads key material from OpenBao directly.

## kmsKey (SSE-KMS)

`POST /api/v1/kmsKeys`:

```json
{
  "name": "finance-sse-kms",
  "description": "Finance department SSE-KMS key",
  "keyMaterial": { "source": "tpm" },
  "rotation": { "enabled": true, "intervalDays": 365 }
}
```

Backed by OpenBao Transit. Convergent dedup preserved within the scope of one `kmsKey`.

## Audit integration

- Keycloak authentication events → forwarded to NovaNas audit log via Keycloak Event Listener SPI → stored in Postgres + `auditPolicy` sinks
- OpenBao audit log → forwarded to same destinations
- Events include: logins, token issuances, failed auth, policy changes, secret reads, key rotations, admin actions

## Bootstrap / DR

See 06 — Boot, Install & Update — bootstrap order:

1. Container runtime (k3s on K8s adapter; dockerd on Docker adapter)
2. Postgres
3. **OpenBao (TPM auto-unseal)**
4. **Keycloak** (reads DB secret from OpenBao)
5. NovaStor (reads master key from OpenBao Transit)
6. novaedge + novanet (configured by NovaNas controllers, not authored as CRDs)
7. novanas-api (Keycloak OIDC client + DB from OpenBao)
8. UI served

DR on new hardware:

1. Fresh install
2. Restore config backup — provides Postgres dump, OpenBao snapshot, sealed unseal keys
3. Admin provides recovery passphrase → decrypts unseal keys → OpenBao unseals → bootstrap continues

## What these replace

| Was | Now |
|---|---|
| better-auth (API session management) | Keycloak OIDC |
| Custom AD/LDAP code in each component | Keycloak federation |
| Custom 2FA implementation | Keycloak built-in |
| Bespoke identity-provider config | `keycloakRealm` API resource |
| Runtime-native Secrets for TLS / DK / tokens | OpenBao paths (mounted via OpenBao Agent at workload admission) |
| Per-component crypto code for DK wrapping | OpenBao Transit |
| cert-manager (optional) | novaedge ACME + OpenBao PKI |
