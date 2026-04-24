# NovaNas OS Build Pipeline

Builds the immutable Debian-based OS image that ships NovaNas on bare metal and
as virtual appliances. Produces signed RAUC update bundles, hybrid-boot ISOs,
and VM-ready disk images.

See `docs/06-boot-install-update.md` for the boot/install/update architecture
and `docs/13-build-and-release.md` for release channels and signing policy.

## Output artifacts

| Artifact | Format | Purpose |
|---|---|---|
| `novanas-<ver>.raucb` | RAUC bundle (verity) | Online A/B updates |
| `novanas-<ver>.iso` | Hybrid ISO (UEFI + BIOS) | Bare-metal install |
| `novanas-<ver>.qcow2` | QEMU disk | KVM / Proxmox |
| `novanas-<ver>.ova` | OVF + tar | VMware |
| `novanas-<ver>.vmdk` | VMware disk | Workstation / Fusion |
| `novanas-<ver>.raw` | raw block device | dd / cloud upload |

v1 ships **amd64 only**. arm64 plumbing stays wired but is not produced.

## Prerequisites

Host tooling required to drive a build (run on Debian/Ubuntu or inside a
privileged build container):

- `mkosi` >= 25 (ships in Debian trixie; `pip install mkosi` elsewhere)
- `systemd-container` (`systemd-nspawn`, used by mkosi)
- `rauc` >= 1.10 (bundle tool)
- `squashfs-tools`, `e2fsprogs`, `dosfstools`
- `xorriso` and `grub-mkrescue` (ISO builder)
- `packer` >= 1.9 (virtual appliances)
- `qemu-system-x86_64`, `qemu-utils` (`qemu-img`)
- `openssl` (for placeholder key generation)

Root/`fakeroot` + loop-mount privileges are required for most targets. Run
inside a privileged container (`--privileged --device /dev/loop-control`) or a
dedicated build VM.

## Build flow

The rootfs is built by [mkosi](https://github.com/systemd/mkosi) from
`os/mkosi.conf` + `os/mkosi.conf.d/*.conf`. Two profiles are declared:

- **live** — rootfs baked into the installer ISO. Includes the journald
  no-rate-limit drop-in for stuck-boot debug visibility.
- **update** — rootfs packed into the A/B RAUC bundle. No debug drop-ins.

```
mkosi --profile=live   build  -> build/out/mkosi/live/image/    (tree)
mkosi --profile=update build  -> build/out/mkosi/update/image/  (tree)
                                     │
build-images.sh --profile=live       │   -> filesystem.squashfs + kernel.*
build-images.sh --profile=update     │   -> rootfs.img + boot.img + kernel.*
                                     │
build-rauc-bundle.sh                 │   -> novanas-<ver>.raucb
build-iso.sh                         │   -> novanas-<ver>.iso
```

### Make targets

```sh
make mkosi-live       # Bootstrap live rootfs tree
make mkosi-update     # Bootstrap update rootfs tree
make live-artifacts   # Squashfs + kernel + initrd
make update-artifacts # ext4 rootfs.img + boot.img
make bundle           # RAUC .raucb (update-artifacts then rauc bundle)
make iso              # Hybrid installer ISO (live-artifacts then grub-mkrescue)
make va               # Virtual appliance images via Packer
make all              # bundle + iso + va
make clean            # Nuke $(BUILD_DIR)
```

Variables:

- `VERSION` (default `0.0.0-dev`) — CalVer `YY.MM.patch` in release builds
- `CHANNEL` (default `dev`) — one of `dev | edge | beta | stable | lts`
- `ARCH` (default `amd64`) — only amd64 is currently produced
- `BUILD_DIR` (default `./build/out`)
- `INSTALLER_BINARY` — path to the A5-Installer static binary (for `make iso`)

Example:

```sh
make all VERSION=26.07.0 CHANNEL=beta INSTALLER_BINARY=../installer/target/release/novanas-installer
```

## Partition layout

The installer creates this on the boot disk (see `installer/` for the code).
RAUC references these partitions by **partlabel**:

```
Disk: /dev/<boot-disk>  (optional mdadm RAID-1 mirror across two disks)
├─ EFI           vFAT    512 MB     partlabel=EFI
├─ Boot          ext4    2   GB     partlabel=Boot    (kernels, initrds, GRUB env)
├─ OS-A          ext4 ro 4   GB     partlabel=OS-A    (RAUC slot A)
├─ OS-B          ext4 ro 4   GB     partlabel=OS-B    (RAUC slot B)
├─ Persistent    ext4    ~80 GB     partlabel=Persistent  (overlays, Postgres, OpenBao, k3s, logs)
└─ Swap          swap    2-8 GB     partlabel=Swap    (optional)
```

Data disks are separate and are never touched by the OS image or the
installer — the chunk engine (`storage/`) owns them.

## Overlayfs

The root filesystem is mounted **read-only** (ext4 `ro` or squashfs). Mutable
paths are overlaid from `/mnt/persistent`:

- `/etc` — overlayed so first-boot and operator edits survive upgrades
- `/var` — overlayed; contains k3s state, journald, container images
- `/home/novanas` — overlayed for admin sessions
- `/mnt/persistent/postgres`, `/opt/openbao/data`, `/var/lib/rancher/k3s` —
  bind-mounted to their canonical locations

See `overlays/persistent.conf` and the `novanas-overlay-*.mount` units.

## A/B updates via RAUC

See `rootfs/etc/rauc/system.conf` and `rauc/manifest.raucm`. Flow:

1. `novanas-updater` pod downloads a signed `.raucb`.
2. RAUC verifies the bundle signature against `/etc/rauc/keyring.pem`.
3. RAUC writes the rootfs image to the **standby** slot (OS-B if A is active).
4. GRUB default is flipped, `novanas-healthcheck.service` is scheduled on next
   boot. A reboot is issued.
5. On boot into the new slot, `novanas-healthcheck.sh` runs: k3s API up,
   `novanas-api` pod Ready, no `CrashLoopBackOff` in `novanas-system`.
6. Success → `rauc status mark-good` → slot becomes primary.
7. Failure → GRUB `countboot` expires after N attempts → auto-rollback.

## First boot

`novanas-firstboot.service` runs `novanas-firstboot.sh` once (guarded by
`ConditionPathExists=!/var/lib/novanas/.firstboot-done`). It:

1. Ensures `/mnt/persistent/*` layout exists.
2. Starts `k3s`, waits for the API.
3. `helm install novanas /opt/novanas/helm/ --namespace novanas-system --create-namespace`.
4. Applies seed CRDs from `/opt/novanas/seed/`.
5. Touches the sentinel file.

After the wizard concludes, the web UI drives user-facing first-run.

## Signing

- **CI builds unsigned bundles** for dev/edge. They cannot be installed on an
  appliance that enforces `/etc/rauc/keyring.pem`.
- **Release signing is offline.** A human operator runs
  `build/sign-release.sh <bundle.raucb>` on an air-gapped workstation holding
  the 2-of-3 custody NovaNas release key (HSM preferred).
- A placeholder self-signed `rauc/cert.pem` and `rauc/keyring.pem` are checked
  in for local build testing **only**. Never ship with them.

Container images and Helm charts use **cosign keyless** (OIDC → Fulcio/Rekor),
handled in `.github/workflows/`, not here.

## Layout

```
os/
├── README.md                (this file)
├── Makefile                 (orchestrates build targets)
├── mkosi.conf               (top-level mkosi configuration)
├── mkosi.conf.d/            (packages, skeleton, profile-gated drop-ins)
├── mkosi.profiles/          (live + update profile declarations)
├── mkosi.postinst           (chroot postinst: k3s, user, hostname, initramfs)
├── mkosi.extra-live/        (files added to live profile only, e.g. journald)
├── rootfs/                  (tree copied into both profiles via ExtraTrees=)
│   ├── etc/...
│   └── usr/local/bin/...
├── rauc/                    (bundle manifest + placeholder keys)
├── overlays/                (overlayfs / mount-unit configuration)
├── build/
│   ├── build-images.sh      (mkosi tree -> squashfs / ext4 / kernel export)
│   ├── build-iso.sh         (thin grub-mkrescue wrapper)
│   ├── build-rauc-bundle.sh (rauc bundle wrapper)
│   └── build-va.sh          (Packer driver)
├── packer/                  (VA templates)
└── tests/smoke-qemu.sh      (QEMU smoke test)
```

### Why a thin ISO wrapper remains

mkosi v25's `Format=disk` produces a UEFI GPT disk image with systemd-boot or
grub installed directly, which is the right answer for cloud VM images but not
for a Debian Live installer ISO — which boots via `boot=live components toram`
and expects `/live/filesystem.squashfs` plus our three-entry grub.cfg (default,
serial, rescue). `build-iso.sh` is ~70 lines around `grub-mkrescue` that takes
the mkosi-built squashfs + kernel + initrd and wraps them into the hybrid
BIOS/UEFI ISO the operator flashes.

## CI integration

Expected to be called by `.github/workflows/release.yml` (not owned by this
scaffold). CI invokes `make bundle iso va VERSION=$TAG CHANNEL=$CHANNEL`
inside a privileged container, then uploads artifacts. Release signing
happens as a separate, human-approved workflow.

## Static validation

The scaffold is validated by:

- `bash -n` on every shell script under `build/` and `rootfs/usr/local/bin/`
- `make -n all` (dry-run of Makefile graph)
- `packer validate packer/novanas.pkr.hcl` (if Packer is installed)
- INI parse of `rauc/manifest.raucm` and `rootfs/etc/rauc/system.conf`

Actual image builds require root + loop devices and are not runnable inside
a sandboxed agent environment.
