# Installer

NovaNas ships a customized Debian installer ISO. Boot order:

1. live-boot/d-i kernel + initrd (from `installer-di/`)
2. d-i runs preseed.cfg (`installer-di/preseed.cfg`)
3. partman uses `partman-auto/expert_recipe` to lay out RAID-1 GPT
4. Base Debian system is installed to slot A (md2)
5. `late_command` (`installer-di/late_command.sh`):
   - configures RAUC (`/etc/rauc/system.conf`)
   - installs the bundled initial NovaNas RAUC bundle
   - copies slot A → slot B for redundancy
6. System reboots into the NovaNas image

## Modes

- **Automatic** (default): Boots straight into install with no prompts. ~15 min.
- **Interactive**: Same preseed but shows each d-i screen so the operator can override (e.g. change keyboard, hostname, mirror).
- **Rescue**: d-i's rescue mode for repairing a broken install.

## Boot menu cmdline

See `installer-di/grub-installer.cfg`.

## Partition layout (per disk, mirrored via partman-md)

| Part | Size | Type | RAID array | Mount on installed system |
|---|---|---|---|---|
| p1 | 1 MiB | BIOS boot | (per-disk, no RAID) | grub embedding |
| p2 | 538 MiB | ESP (vfat) | md0 | /boot/efi |
| p3 | 1024 MiB | ext4 | md1 | /boot |
| p4 | 4096 MiB | ext4 | md2 | / (RAUC slot A) |
| p5 | 4096 MiB | ext4 | md3 | /mnt/os-b (RAUC slot B) |
| p6 | rest | ext4 | md4 | /var/lib/novanas |

## Build pipeline

The ISO is built by `installer-di/build-installer-iso.sh`, called from
`os/build/build-iso.sh` (thin wrapper). Pipeline stages:

1. Download `debian-trixie-netinst.iso` (cached locally, sha256 verified if pinned)
2. Extract ISO contents to a working directory
3. Inject `preseed.cfg` into `install.amd/initrd.gz` (cpio repack)
4. Copy `late_command.sh` + initial RAUC bundle + signing keyring into `/cdrom/novanas/`
5. Replace GRUB and isolinux menus to default to "automatic install"
6. Rebuild as a hybrid BIOS+EFI bootable ISO via xorriso

CI runs the pipeline inside a `debian:trixie-slim` container with
`xorriso`, `cpio`, `gunzip`, and `curl` installed.
