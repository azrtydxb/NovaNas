# Disaster recovery

Recovering from a full-box failure — chassis dead, motherboard
unrecoverable, fire/flood, hardware stolen. Covers importing a pool on
new hardware and restoring configuration.

## Scope

This procedure assumes:

- The pool disks survived. If they did not, skip to
  `offsite-replication.md` — restore from the remote copy.
- You have a current config backup (see `ConfigBackupPolicy`).
- You have fresh hardware of compatible spec (same HBA family, ≥ same
  CPU/RAM).

## RPO / RTO expectations

- **RPO**: up to one `ConfigBackupPolicy.spec.schedule` interval +
  `ReplicationJob.spec.schedule` for the most-recent dataset. Plan for
  24h worst case unless you have tighter schedules configured.
- **RTO**: ~2h for a single-chassis rebuild once hardware is on-site —
  most of that is pool scrubbing after import.

## 0. Before disaster — prerequisites

Work through this list *now*, not during the incident:

- [ ] `ConfigBackupPolicy` runs at least nightly, shipping to a target
      that is *not* the local pool.
- [ ] Off-site replication covers every dataset you can't afford to
      lose (see `offsite-replication.md`).
- [ ] A second copy of the encryption recovery key lives somewhere
      off-site (safe-deposit box, other admin's password manager).
- [ ] Hardware spares for the HBA and SFPs are on the shelf, or a
      known-good supplier SLA is signed.
- [ ] This document, the config backup URL, and the OpenBao unseal
      procedure are printed and kept off-line.

## 1. Acquire and rack new hardware

Match the original chassis as closely as possible. Critical:

- **Same HBA family** — cross-vendor import (e.g., Broadcom → Adaptec)
  requires a full drive re-signature. Stay within vendor.
- **≥ same RAM** — required for in-memory chunk index fit.
- **Same CPU arch** — x86_64 ↔ arm64 pool import is not supported.

Install the NovaNas OS using the installer USB. Do *not* let it create
any pool yet.

## 2. Transport and install the disks

- Pull disks from the dead chassis in order; label slot numbers so you
  can return them to the same bay positions (not strictly required,
  but makes life easier if any ever needs replacing).
- Seat them in the new chassis. Power on.

Verify the OS sees every disk:

```sh
novanasctl disk list
# Expect every original disk with status=Unassigned and
# a valid NovaNas pool signature in the "member-of" annotation.
```

## 3. Restore configuration

Fetch the latest config backup:

```sh
novanasctl system config restore \
  --from <cloudbackup-target> \
  --bundle latest \
  --dry-run
```

Inspect the diff. When satisfied, apply:

```sh
novanasctl system config restore \
  --from <cloudbackup-target> \
  --bundle latest \
  --confirm
```

This recreates CRDs (`StoragePool`, `Dataset`, `Share`, `User`, etc.)
but does *not* touch the raw disks yet.

## 4. Import the pool

```sh
novanasctl pool import <pool-name> --confirm
```

The import:
- Reads the pool signature from a quorum of disks.
- Verifies the pool UUID matches the restored `StoragePool` CRD.
- Replays the journal to a consistent state.
- Starts an implicit scrub in the background.

Monitor:

```sh
novanasctl pool get <pool-name> -w
```

`status.phase` should move `Importing → Ready` within minutes for a
clean pool. A dirty pool (power loss during write) can take longer.

## 5. Unseal OpenBao

The identity/secrets layer is the last thing to come back online:

```sh
# If TPM unseal is configured and the TPM survived (i.e., same chassis
# TPM), this happens automatically.
novanasctl system identity status

# Otherwise, shamir unseal with the offline-stored keys:
openbao operator unseal <key-1>
openbao operator unseal <key-2>
openbao operator unseal <key-3>
```

## 6. Verify data

Before opening to users:

```sh
# Scrub completes cleanly.
novanasctl pool get <pool-name> -o json | jq '.status.scrub'

# Sample a dozen files and checksum against a known-good list.
for f in $(novanasctl dataset sample <dataset> --count 12); do
  sha256sum "$f"
done

# Verify replication is reconnecting.
novanasctl replication job list
```

## 7. Open to users

```sh
novanasctl share resume --all
novanasctl system maintenance exit
```

Announce that DR is complete. File an RCA within 48h.

## Gotchas

- **HBA firmware mismatch.** Broadcom LSI 9400 vs 9500 may present
  disks with slightly different sector geometry reporting. Import
  works, but scrub can throw false-positive checksum errors for the
  first hour. Let it complete; repeat scrub — if errors persist,
  escalate.
- **TPM lost.** A destroyed TPM means the auto-unseal key is gone and
  you must fall back to shamir unseal. This is why the off-line
  shamir keys matter.
- **Clock skew.** If the new chassis' RTC is way off, replication TLS
  handshakes fail. `timedatectl set-ntp true` and wait for sync before
  resuming replication jobs.
- **Wrong config bundle.** A "latest" bundle from the backup target is
  not necessarily from the primary cluster — confirm the bundle's
  cluster ID before `--confirm`.
