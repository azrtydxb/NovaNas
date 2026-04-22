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

- `mmdebstrap` >= 1.4
- `debootstrap` (fallback)
- `rauc` >= 1.10 (bundle tool)
- `squashfs-tools`, `e2fsprogs`, `dosfstools`
- `xorriso` and `grub-mkrescue` (ISO builder)
- `packer` >= 1.9 (virtual appliances)
- `qemu-system-x86_64`, `qemu-utils` (`qemu-img`)
- `ovftool` (optional, for `.ova` output) or fallback tar+OVF
- `skopeo` or `crane` (container image pre-pull)
- `openssl` (for placeholder key generation)

Root/`fakeroot` + loop-mount privileges are required for most targets. Run
inside a privileged container (`--privileged --device /dev/loop-control`) or a
dedicated build VM.

## Build targets

```sh
make base       # Debian minimal rootfs via mmdebstrap -> build/base-rootfs.tar
make layered    # Add NovaNas layer (k3s, Helm chart, pre-pulled images)
make bundle     # RAUC .raucb (unsigned by CI; signed offline for release)
make iso        # Hybrid bootable ISO including installer + RAUC bundle
make va         # All virtual appliance images via Packer
make all        # base -> layered -> bundle -> iso -> va
make clean      # Nuke $(BUILD_DIR)
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
├── mmdebstrap.conf          (base Debian build configuration)
├── recipes/
│   ├── base.yaml            (debos-compatible base recipe)
│   └── layered.yaml         (NovaNas layer on top of base)
├── rootfs/                  (tree copied into the image)
│   ├── etc/...
│   └── usr/local/bin/...
├── rauc/                    (bundle manifest + placeholder keys)
├── overlays/                (overlayfs / mount-unit configuration)
├── build/                   (build scripts)
├── packer/                  (VA templates)
└── tests/smoke-qemu.sh      (QEMU smoke test)
```

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
