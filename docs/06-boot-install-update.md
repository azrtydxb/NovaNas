# 06 — Boot, Install & Update

## OS base

**Immutable Debian** — custom-built minimal Debian (debootstrap / mmdebstrap) with:

- Read-only root filesystem (ext4 mounted `ro` or squashfs)
- Overlayfs for `/etc`, `/var`, `/home-admin` backed by persistent partition
- A/B partition scheme managed by **RAUC**
- Atomic updates via signed RAUC bundles
- Automatic rollback on failed boot (GRUB handles the flip)

Not CoreOS, not Elemental, not Fedora-based — Debian for familiar ecosystem and TrueNAS-parallel operational model.

## Partition layout

```
/dev/<boot-disk>  (+ optional mdadm RAID-1 mirror on second boot disk)
├─ EFI           (vFAT,  512 MB)
├─ Boot          (ext4,  2 GB)
├─ OS-A          (ext4/squashfs ro, 4 GB)   ← active slot
├─ OS-B          (ext4/squashfs ro, 4 GB)   ← standby / update target
├─ Persistent    (ext4,  ~80 GB)            ← /etc overlay, /var, Postgres, OpenBao, logs, k3s state, RAUC state
└─ Swap          (optional, 2-8 GB)
```

Data disks are separate, untouched by the installer, exclusively managed by the chunk engine.

## Boot disk redundancy

Optional **mdadm RAID-1** across two boot disks for operators who want it. Default install uses a single boot disk — persistent data is recoverable via **config backup + restore** onto a fresh install.

## Installer

**Text-mode curses** installer (Rust + `ratatui` or Go + `bubbletea`), designed for broad hardware compatibility:

1. Language + keyboard layout
2. Select OS disk(s), confirm wipe
3. Optional: set static IP or accept DHCP
4. Install RAUC OS image to `OS-A`, write GRUB, create partitions
5. Reboot into the newly-installed system

## First boot wizard (web UI)

After install, user points browser at `https://nas.local` or displayed IP:

1. Language + timezone
2. Admin user + password + 2FA enrollment (TOTP/WebAuthn via Keycloak)
3. Hostname
4. Network confirm (DHCP or static; already configured from installer)
5. Identity provider (optional: AD join, LDAP bind, or skip — local users only)
6. Disks detected → offer to create first StoragePool
7. Optional: create first Dataset + first Share (or skip to dashboard)
8. Update channel selection (stable / beta / dev)
9. Telemetry opt-in/out
10. Done → main dashboard

Every step is skippable. Expert path: skip 5–9, configure manually from main UI.

## Virtual appliance images

Shipped alongside ISO from day one — same base rootfs, different container:

- `.ova` — VMware
- `.qcow2` — KVM / Proxmox
- `.vmdk` — VMware Workstation / Fusion
- `.raw` — dd-to-device, cloud uploads
- Vagrant box — dev-only

Built via Packer driving KVM, converted to each output format.

## Updates

Atomic A/B via RAUC.

### Update channels

| Channel | Cadence | Audience |
|---|---|---|
| `dev` | Every main commit | Internal, early adopters willing to break |
| `edge` | Same as `dev` (alias for per-commit) | Same |
| `beta` | Monthly tag on release branch | Willing to test before stable |
| `stable` | Quarterly major, monthly patch | Default for production |
| `lts` | Long-term-support for a release branch | Enterprise customers wanting stability |

### Update flow

1. `novanas-updater` pod polls release server on cadence per `UpdatePolicy`
2. New bundle available → download signed RAUC `.raucb`
3. RAUC verifies signature against embedded NovaNas public key
4. RAUC writes bundle to standby partition (`OS-B` if A is active)
5. Flip bootloader default to standby; reboot
6. First boot on new partition: health check runs
7. Pass → mark slot "good", previous slot becomes standby for next update
8. Fail → GRUB's `countboot` counter expires → auto-rollback to previous slot
9. Operator applies Helm chart version shipped in bundle (schema migrations run here)

### updatePolicy

`PUT /api/v1/updatePolicy`:

```json
{
  "channel": "stable",
  "automatic": true,
  "maintenanceWindow": { "cron": "0 3 * * SUN", "durationMinutes": 60 },
  "requireQuorumOfDisksHealthy": true,
  "snapshotBeforeUpdate": true
}
```

## Factory reset

Three tiers accessible from the UI:

| Tier | Wipes | Use case |
|---|---|---|
| **Soft** | Admin password, network config, identity providers | Forgotten password, misconfig |
| **Config** | All API resources (back to post-install-wizard state). Data pools preserved. | Start config over, keep data |
| **Full** | OS + persistent + data disks (secure erase) | Sell, recycle, retire |

### Secure erase priority

1. If encryption is enabled: destroy all DKs in OpenBao + overwrite superblocks → cryptographic erase (milliseconds, any device)
2. NVMe: `nvme format --ses=1` (crypto erase) or `--ses=2` (user data erase)
3. SATA SSD: `hdparm --security-erase`
4. SATA HDD: `blkdiscard` + single-pass zero write (option to go multi-pass "paranoid")

## Config backup & restore

Daily by default, multi-destination.

### configBackupPolicy

`PUT /api/v1/configBackupPolicy`:

```json
{
  "cron": "0 1 * * *",
  "retention": { "keepDaily": 30 },
  "passphraseSecret": "config-backup-passphrase",
  "destinations": [
    { "type": "dataset", "dataset": "backups" },
    { "type": "cloud",   "target": "offsite-s3" },
    { "type": "email",   "channel": "admin-email" }
  ]
}
```

### What a backup contains

- Full Postgres dump — the API server's source of truth for every NovaNas resource (pools, datasets, shares, apps, VMs, networking, identity, alerting, settings)
- Postgres dump also covers Keycloak realm, audit log history, user DB
- OpenBao snapshot
- Sealed unseal keys (encrypted with user passphrase — the only way to recover an OpenBao instance on new hardware)
- Disk pool GUIDs and CRUSH map reference (for restore to find pools on re-imported disks)
- Runtime adapter state hint: which runtime backend (k3s / docker) the appliance was running, so restore can re-provision it identically

There is no CRD export and no `kubectl get` step — the runtime holds no source-of-truth state.

### Restore flow

1. Fresh install on new hardware
2. First-boot wizard offers "Restore from config backup"
3. Admin uploads backup archive + passphrase
4. Wizard restores in order: Postgres → OpenBao (unseals with passphrase) → Keycloak realm → NovaNas API server starts and rehydrates controllers from API state → runtime adapter brings up workloads
5. Pool import: wizard detects NovaNas superblocks on data disks, matches GUIDs from backup, auto-imports
6. Encryption keys unseal automatically once OpenBao is restored
7. System resumes with shares, users, schedules, apps all active

## Bootstrap order at every boot

1. Bootloader → kernel → init (systemd)
2. Host services: nmstate, networkd, journald, SSH (if enabled)
3. k3s starts (embedded sqlite or etcd)
4. Postgres pod starts (hostPath on persistent partition)
5. **OpenBao pod starts, TPM auto-unseals** via measured-boot PCR binding
6. Keycloak pod starts, reads DB credentials from OpenBao
7. novanet + novaedge pods start
8. NovaStor meta + agents + dataplane start; chunk engine fetches master key from OpenBao Transit
9. novanas-api starts; Keycloak OIDC client + DB creds from OpenBao
10. novanas-ui served via novaedge at `https://nas.local`
11. Apps and VMs per `autostart` directives

## Disaster scenarios

| Scenario | Recovery path |
|---|---|
| OS disk dies, data disks fine | Fresh install → restore config backup → pool auto-imports |
| Data disk dies | Hot-swap; chunk engine rebuilds; see `07-disk-lifecycle` |
| Both boot disks die (no mirror) | Same as OS disk dies |
| User pulls all disks, reinserts in wrong slots | Works — disks identified by WWN, not slot |
| Firmware update bricks box | A/B rollback via GRUB countboot; if bootloader corrupt, reflash USB |
| Factory reset while encrypted | Full reset explicitly destroys locks/keys; documented as only escape for compliance-locked data |
