# NovaNas Installer

Text-mode curses installer (Go + [bubbletea](https://github.com/charmbracelet/bubbletea))
that runs from the NovaNas live ISO and installs the OS to target disks.

## Scope

A single-purpose utility that walks the operator through:

1. Language + keyboard layout
2. Timezone
3. OS disk selection (1 disk or 2-disk mdadm RAID-1)
4. Network (DHCP or static)
5. Confirm
6. Install (partition, format, extract RAUC bundle, install GRUB, seed persistent partition)
7. Reboot

Data disks are never touched — they are managed post-install by the chunk engine.
First-boot configuration (admin user, pool creation, IdP, ...) happens in the web UI.

## Runtime dependencies

These tools must be present in the live environment the installer runs in:

- `lsblk` (util-linux)
- `parted`
- `mkfs.vfat` (dosfstools), `mkfs.ext4` (e2fsprogs)
- `mdadm` (only if RAID-1 requested)
- `grub-install` + `grub-mkconfig` (grub-efi-amd64)
- `unsquashfs` (squashfs-tools) — for RAUC bundle extraction

The A5-OS ISO bakes all of these in.

## Invocation

```sh
novanas-installer --bundle /cdrom/novanas.raucb
```

Flags:

| Flag | Meaning |
|---|---|
| `--bundle PATH` | Path to the signed RAUC bundle to install (default `/cdrom/novanas.raucb`) |
| `--debug` | Verbose logging to stderr, shows extra TUI diagnostics |
| `--skip-network` | Skip the network step (will fall back to DHCP on first boot) |
| `--auto-disk=/dev/sdX` | Unattended: pick this disk non-interactively |
| `--i-am-sure` | REQUIRED to actually write partition tables. Without it, the install step runs in dry-run mode. |

## Dev run

```sh
GOWORK=off go run . --debug
```

Without `--i-am-sure`, the install step is dry-run and prints the commands it *would* run.

## Building

```sh
make build         # -> bin/novanas-installer (static binary)
make test          # unit tests
make vet
```

The Dockerfile builds a runtime image containing the installer plus its external
dependencies, used by the A5-OS ISO build pipeline.

## Logging

All actions append to `/var/log/novanas-installer.log` (inside the live environment).
With `--debug`, the same log is also echoed to stderr.

## Status / TBDs

- RAUC bundle signature verification is stubbed (`install/rauc.go`). Real signing
  lands with the release-signing infrastructure (wave 6+).
- Only English strings are shipped in v1; the language step records the chosen
  locale for future translations.
- Swap partition is currently skipped (docs/06 calls it optional); add later.
