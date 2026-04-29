# ZFS Native Encryption with TPM-Sealed Key Escrow

NovaNAS protects encrypted ZFS datasets with a two-layer key hierarchy:

1. ZFS native encryption (`encryption=aes-256-gcm`, `keyformat=raw`) holds
   the actual data-encryption key in-kernel.
2. The 32-byte raw ZFS key is **TPM-sealed** via the same envelope
   construction used by `nova-bao-unseal` and `nova-kdc-unseal`
   (`internal/host/tpm.WrapAEAD`/`UnwrapAEAD`), then **escrowed** in
   OpenBao under `nova/zfs-keys/<encoded-dataset-name>`.

The wrapped blob is opaque to OpenBao: only a process running on the
**same physical host with the same boot state** can ask the TPM to
unseal it. An attacker who exfiltrates an OpenBao backup learns
nothing about the dataset's plaintext.

## Architecture

```
Operator -> nova-api -> tpm.WrapAEAD(rawKey)  -> OpenBao (nova/zfs-keys/<ds>)
                    -> zfs create -o encryption=aes-256-gcm
                                  -o keyformat=raw
                                  -o keylocation=prompt
                                  <ds>           (rawKey fed via stdin)

Boot     -> nova-zfs-keyload.service:
            for each escrowed dataset:
              wrapped = OpenBao.Get("nova/zfs-keys/<ds>")
              rawKey  = tpm.UnwrapAEAD(wrapped)
              zfs load-key -L prompt <ds>           (rawKey via stdin)
```

The raw key never appears on disk in plaintext, never appears in
process argv, never appears in any log, and never leaves the host
unless the operator explicitly invokes the recovery API.

## API

### Create an encrypted dataset

```http
POST /api/v1/datasets/tank%2Fsecret/encryption
Content-Type: application/json
{
  "type": "filesystem",
  "algorithm": "aes-256-gcm",
  "properties": { "compression": "lz4" }
}
```

The server:

1. Generates a fresh 32-byte raw key with `crypto/rand`.
2. TPM-seals it with the boot-state PCR policy.
3. Writes the wrapped blob to `nova/zfs-keys/tank__2Fsecret`.
   (Slashes are preserved as path separators in the secret key; `.`,
   `:`, and other ZFS-legal-but-secret-key-illegal characters are
   percent-encoded with `_` substituted for `%`. See
   `internal/host/zfs/dataset.EncodeSecretKey`.)
4. Runs `zfs create -o encryption=aes-256-gcm -o keyformat=raw -o
   keylocation=prompt tank/secret` and pipes the raw key on stdin.

Returns `201 Created` with `{ dataset, algorithm, created }`.

Required permission: `nova:encryption:write`.

### Load / unload keys

```http
POST /api/v1/datasets/tank%2Fsecret/encryption/load-key
POST /api/v1/datasets/tank%2Fsecret/encryption/unload-key
```

Both return `204 No Content` on success. Load fetches the wrapped
blob, TPM-unseals, and feeds `zfs load-key -L prompt tank/secret` over
stdin. Unload runs `zfs unload-key tank/secret` and leaves the wrapped
blob in OpenBao.

Required permission: `nova:encryption:write`.

### Boot-time auto-load

`nova-zfs-keyload.service` does the same load-key dance for every
dataset under `nova/zfs-keys/` at every boot. It runs after
`openbao.service` (so we can fetch wrapped blobs) and after
`nova-bao-tpm-unseal.service` (so OpenBao is unsealed), and **before**
any consumer of the data: `zfs-mount.service`, `nova-api.service`,
`nfs-server.service`, `smbd.service`, `nova-iscsi-restore.service`,
`nova-nvmet-restore.service`. Those units `Requires=` it, so a key-
load failure prevents them from starting.

A PCR-mismatch failure (boot state changed since seal) does **not**
trigger a restart loop; it fails closed and surfaces in journalctl:

```
journalctl -u nova-zfs-keyload.service
```

### Recovery (break-glass)

```http
POST /api/v1/datasets/tank%2Fsecret/encryption/recover
```

Returns `200 OK`:

```json
{
  "dataset": "tank/secret",
  "keyHex": "<64-char hex of the raw 32-byte ZFS key>"
}
```

Required permission: `nova:encryption:recover` (admin-only).

Every call writes an audit-log row with `action=encryption.recover`,
`target=<dataset>`, `result=accepted|rejected`, the caller identity,
and a timestamp. The actual recovered key is **not** in the audit
payload.

Use cases:

- **Migrating a dataset to a different host.** Save the hex key
  somewhere safe, `zfs send -w` the dataset to the new host, then on
  the new host pipe the hex (decoded back to 32 bytes) into
  `zfs load-key -L prompt`. Subsequent re-escrow on the new host (via
  `POST .../encryption` with the existing key — TODO endpoint) seals
  the same key under that host's TPM.
- **Cold backup of the key.** Print it, sign it, lock it in a safe.
  If your NAS dies and the backup TPM is unrecoverable, the dataset
  is still recoverable from a `zpool import` + manual `zfs load-key`.
- **Forensic / compliance dumping.** Dataset auditors who need
  plaintext access can request the key without unsealing every
  service on the box.

## Key rotation

ZFS supports rekeying via `zfs change-key`. The current API exposes
the legacy `/datasets/{full}/change-key` endpoint, which accepts the
new key parameters as ZFS properties. To rotate while preserving
escrow:

1. Generate a new raw key client-side (32 bytes from a CSPRNG).
2. Call `POST /datasets/{full}/change-key` with the new
   `keylocation`/`keyformat`. (TODO: bake stdin-aware rotation into
   `EncryptionManager`.)
3. Re-escrow by calling `POST /datasets/{full}/encryption` with the
   new key.

Until the rotate-with-escrow endpoint lands, do not change keys
manually — the OpenBao escrow will go stale and `nova-zfs-keyload`
will refuse to load on next boot.

## Threat model

| Adversary capability | Outcome |
|---|---|
| Steal disks | ZFS native encryption protects data at rest. |
| Steal disks + OpenBao backup | Wrapped blob is useless without TPM. |
| Steal disks + OpenBao + TPM | Operator must shred the TPM (`tpm2_clear`) or destroy the chip. |
| Replace boot loader / kernel | PCR change → unseal fails → keyload service fails closed → consumers don't start. |
| Compromise nova-api at runtime | Has access to recovery endpoint behind admin RBAC; calls are audit-logged. |
| Compromise nova-zfs-keyload at boot | Has unwrapped key in memory just like ZFS itself; same risk surface as any ZFS encryption deployment. |

## File map

- `internal/host/tpm/envelope.go` — TPM envelope (`WrapAEAD`/`UnwrapAEAD`).
- `internal/host/zfs/dataset/encryption.go` — `EncryptionManager`, `CreateSpec.EncryptionEnabled`, secret-key encoding.
- `cmd/nova-zfs-keyload/main.go` — boot-time loader.
- `deploy/systemd/nova-zfs-keyload.service` — systemd unit.
- `internal/api/handlers/encryption.go` — HTTP handler with recovery audit.
- `internal/auth/rbac.go` — `PermPoolEncryption{Read,Write,Recover}`.
- `clients/go/novanas/encryption.go` — Go SDK.
