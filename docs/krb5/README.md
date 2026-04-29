# Embedded MIT KDC

NovaNAS optionally runs an embedded MIT Kerberos KDC on the NAS host so
NFSv4 with `sec=krb5*` can be brought up without operating a separate
identity server. This document covers bootstrap, the systemd unit graph,
the master-key threat model, and the principal management API.

## Identity model

We issue **service principals only** — `nfs/<host>@REALM`,
`host/<host>@REALM`, `nfs/<k3s-node>@REALM`. Per-user principals are
**not** minted by NovaNAS. Keycloak remains the source of truth for
human identities; user authentication continues to flow through OIDC.

Default realm is `NOVANAS.LOCAL`. Operators that want a different name
set `NOVA_KDC_REALM` in `/etc/nova-kdc/bootstrap.env` before first boot.

## Files and units

| Path | Purpose |
|------|---------|
| `/var/lib/krb5kdc/principal` | LMDB principal database |
| `/etc/nova-kdc/master.enc` | TPM-sealed master key (mode 0600 root) — the only persistent copy |
| `/run/krb5kdc/.k5.<REALM>` | Plaintext stash on tmpfs (mode 0600 root) — materialized at boot, never on disk |
| `/var/lib/krb5kdc/kdc.conf` | KDC server config (rendered from template; `key_stash_file` points at `/run`) |
| `/var/lib/krb5kdc/kadm5.acl` | kadmind ACL |
| `/etc/nova-kdc/master.pw` | Master password used at create time (mode 0600) |
| `/etc/nova-kdc/bootstrap.env` | Operator overrides for `nova-kdc-bootstrap.sh` |

systemd units:

- `nova-kdc-bootstrap.service` (oneshot) — runs `nova-kdc-bootstrap.sh`
  if `/var/lib/krb5kdc/principal` is missing. Creates the DB + stash,
  TPM-seals the master key (default), shreds the plaintext, and
  bootstraps the admin and host principals.
- `nova-kdc-unseal.service` (oneshot) — at every boot, decrypts
  `/etc/nova-kdc/master.enc` via the TPM and writes the plaintext stash
  to `/run/krb5kdc/.k5.<REALM>` (tmpfs). Ordered `Before=krb5kdc.service`
  so the daemon never starts without an unsealed stash.
- `krb5kdc.service` — the KDC daemon. `Requires=nova-kdc-unseal.service`
  and `Wants=nova-kdc-bootstrap.service`, gated on the database file.
- `kadmind.service` — admin server, `Requires` krb5kdc and the unseal
  oneshot.

## Bootstrap

```bash
sudo install -d -m 700 /etc/nova-kdc
head -c 32 /dev/urandom | base64 | sudo tee /etc/nova-kdc/master.pw >/dev/null
sudo chmod 600 /etc/nova-kdc/master.pw

# Optional: override realm/hostname before first boot.
echo 'NOVA_KDC_REALM=NOVANAS.LOCAL' | sudo tee /etc/nova-kdc/bootstrap.env

sudo systemctl enable --now nova-kdc-bootstrap.service
sudo systemctl enable --now krb5kdc.service kadmind.service
```

The bootstrap script is idempotent — re-running it on a system that
already has `/var/lib/krb5kdc/principal` is a no-op.

## Master-key threat model

The KDC's master key encrypts every principal key in the database.
NovaNAS TPM-seals it by default — the same pattern used for OpenBao's
unseal keys (`cmd/nova-bao-unseal/`).

### How TPM sealing works

1. At bootstrap, `kdb5_util create -s` produces a plaintext stash at
   `/var/lib/krb5kdc/.k5.<REALM>`.
2. `nova-kdc-unseal --init` reads that stash, generates a fresh AES-256
   data-encryption key (DEK), TPM-seals the DEK against PCR 7 (the
   UEFI secure-boot state), AES-GCM-encrypts the stash with the DEK,
   and writes the result to `/etc/nova-kdc/master.enc` (mode 0600 root).
3. The bootstrap script `shred -u`s the plaintext stash. From this
   point on the master key never exists on persistent storage.
4. At every boot, `nova-kdc-unseal.service` runs before
   `krb5kdc.service`. It reads `/etc/nova-kdc/master.enc`, asks the TPM
   to unseal the DEK (which only succeeds if PCR 7 still matches),
   AES-GCM-decrypts the stash, and writes it to
   `/run/krb5kdc/.k5.<REALM>` (tmpfs, mode 0600 root).
5. `krb5kdc` and `kadmind` open the runtime stash from `/run` per
   `kdc.conf`. On reboot, tmpfs is wiped and the cycle repeats.

### What happens if PCRs change

If the boot chain is altered (firmware update, secure-boot toggle,
bootloader replacement, etc.), the TPM refuses to unseal the DEK and
`nova-kdc-unseal` exits non-zero with a clear log line:

```
PCR mismatch: boot state may have changed since seal
```

`krb5kdc.service` will then refuse to start (`Requires=` ordering).
Recovery: re-bootstrap from a backup of `/etc/nova-kdc/master.pw` (or
re-create the realm) — the master password regenerates the stash, and
`nova-kdc-unseal --init` re-seals it against the new PCR state.

### Migrating an existing KDC

For installations that came up before TPM sealing landed (plaintext
stash at `/var/lib/krb5kdc/.k5.<REALM>`):

```bash
sudo nova-kdc-unseal --init \
     --realm NOVANAS.LOCAL \
     --blob /etc/nova-kdc/master.enc \
     --input /var/lib/krb5kdc/.k5.NOVANAS.LOCAL

sudo install -m 600 -o root -g root \
     /var/lib/krb5kdc/.k5.NOVANAS.LOCAL \
     /run/krb5kdc/.k5.NOVANAS.LOCAL

sudo shred -u /var/lib/krb5kdc/.k5.NOVANAS.LOCAL

# Update kdc.conf to point at /run
sudo sed -i 's|key_stash_file *=.*|key_stash_file = /run/krb5kdc/.k5.NOVANAS.LOCAL|' \
     /var/lib/krb5kdc/kdc.conf

sudo systemctl daemon-reload
sudo systemctl restart krb5kdc kadmind
```

### Fallback: no-TPM mode

For hosts without a usable TPM, set `NOVA_KDC_TPM_SEAL=0` in
`/etc/nova-kdc/bootstrap.env` before first boot. The bootstrap script
then keeps the plaintext stash at `/var/lib/krb5kdc/.k5.<REALM>` and
the operator must edit `kdc.conf` to point `key_stash_file` at that
persistent path. In this mode an attacker with root on the NAS can
extract every service key — treat the NAS host as a Tier-0 asset.

## Principal management API

All paths below are under `/api/v1`. Authentication is the standard
Keycloak JWT used by the rest of nova-api. Permissions:

- `nova:krb5:read` — list/get principals, read KDC status
- `nova:krb5:write` — create/delete principals, fetch keytabs (the
  keytab response is the cryptographic credential, so write-tier auth)

### KDC status

```bash
curl -sH "Authorization: Bearer $TOKEN" \
  https://nas.example.com/api/v1/krb5/kdc/status
```

```json
{
  "running": true,
  "realm": "NOVANAS.LOCAL",
  "databaseExists": true,
  "stashSealed": true,
  "principalCount": 4
}
```

### List principals

```bash
curl -sH "Authorization: Bearer $TOKEN" \
  https://nas.example.com/api/v1/krb5/principals
```

### Create a service principal (random key, typical case)

```bash
curl -sH "Authorization: Bearer $TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"name":"nfs/node1.lab.example.com","randkey":true}' \
     https://nas.example.com/api/v1/krb5/principals
```

### Create with an initial password

```bash
curl -sH "Authorization: Bearer $TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"name":"alice","password":"correct horse battery staple"}' \
     https://nas.example.com/api/v1/krb5/principals
```

### Get one

```bash
curl -sH "Authorization: Bearer $TOKEN" \
  "https://nas.example.com/api/v1/krb5/principals/nfs%2Fnode1.lab.example.com"
```

(Note `/` → `%2F`.)

### Delete (idempotent)

```bash
curl -X DELETE -H "Authorization: Bearer $TOKEN" \
  "https://nas.example.com/api/v1/krb5/principals/nfs%2Fnode1.lab.example.com"
```

### Fetch a keytab

`POST` because it has a side effect — `ktadd` rotates the principal's
KVNO. Existing keytabs distributed to clients become invalid; only the
returned bytes are valid going forward.

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
     -o node1.keytab \
     "https://nas.example.com/api/v1/krb5/principals/nfs%2Fnode1.lab.example.com/keytab"
```

The response is `application/octet-stream`. The first byte is `0x05`
(MIT keytab magic). Distribute atomically (e.g. `mv` into place after
`fsync`).

## Programmatic access (Go SDK)

```go
import "github.com/novanas/nova-nas/clients/go/novanas"

c, _ := novanas.New(novanas.Config{BaseURL: "https://nas.example.com", Token: tok})

st, _ := c.GetKDCStatus(ctx)
_ = c.CreatePrincipal(ctx, novanas.CreatePrincipalSpec{Name: "nfs/n1", Randkey: true})
keytab, _ := c.GetPrincipalKeytab(ctx, "nfs/n1")
```
