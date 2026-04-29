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
| `/var/lib/krb5kdc/.k5.<REALM>` | Master-key stash file (mode 0600 root) |
| `/var/lib/krb5kdc/kdc.conf` | KDC server config (rendered from template) |
| `/var/lib/krb5kdc/kadm5.acl` | kadmind ACL |
| `/etc/nova-kdc/master.pw` | Master password used at create time (mode 0600) |
| `/etc/nova-kdc/bootstrap.env` | Operator overrides for `nova-kdc-bootstrap.sh` |

systemd units:

- `nova-kdc-bootstrap.service` (oneshot) — runs `nova-kdc-bootstrap.sh`
  if `/var/lib/krb5kdc/principal` is missing. Creates the DB + stash,
  bootstraps the admin and host principals.
- `krb5kdc.service` — the KDC daemon. `Wants` the bootstrap unit, gated
  on the database file existing.
- `kadmind.service` — admin server, `Requires` krb5kdc.

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
NovaNAS v1 stores it in a stash file at `/var/lib/krb5kdc/.k5.<REALM>`
with mode 0600 owned by root. Anyone with root on the NAS host can read
the master key; this is consistent with how distros ship MIT krb5 by
default.

**Known follow-up** (tracked separately): wire the KDC stash through
the same TPM-sealing pattern used for OpenBao
(`deploy/openbao/setup-openbao.sh` + `cmd/nova-bao-unseal/`). The
intended shape is:

1. At bootstrap, generate the stash, encrypt with a TPM-sealed key.
2. At boot, a `nova-kdc-tpm-unseal.service` oneshot decrypts the stash
   into a tmpfs path before `krb5kdc.service` starts.
3. The plaintext stash never lands on persistent storage.

Until this lands, an attacker with root on the NAS can extract every
service key. Operators should treat the NAS host as a Tier-0 asset and
restrict console + SSH access accordingly.

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
