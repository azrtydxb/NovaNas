# Current Installer Inventory: Actions & d-i Mapping

**Date:** 2026-04-25  
**Purpose:** Enumerate all installer actions from the current bubbletea-based Go installer (`installer/` directory) to serve as a reference for writing the debian-installer preseed file, partman recipe, and `late_command.sh`.

## Step 1: Current Installer Actions by Module

### 1. Disk Partitioning (`installer/internal/disks/partition.go`)

- Runs `parted --script <device> mklabel gpt` to create a GPT partition table
- Creates EFI partition: `parted --script <device> mkpart EFI fat32 1MiB 513MiB` and sets ESP flag with `parted --script <device> set 1 esp on`
- Creates Boot partition: `parted --script <device> mkpart Boot ext4 513MiB 2561MiB`
- Creates OS-A partition: `parted --script <device> mkpart OS-A ext4 2561MiB 6657MiB`
- Creates OS-B partition: `parted --script <device> mkpart OS-B ext4 6657MiB 10753MiB`
- Creates Persistent partition: `parted --script <device> mkpart Persistent ext4 10753MiB 92161MiB` (consuming up to 80 GiB with 1 MiB lead-in alignment)
- Formats EFI partition: `mkfs.vfat -F 32 -n EFI <p1>`
- Formats Boot partition: `mkfs.ext4 -F -L boot <p2>`
- Formats OS-A partition: `mkfs.ext4 -F -L os-a <p3>`
- Formats OS-B partition: `mkfs.ext4 -F -L os-b <p4>`
- Formats Persistent partition: `mkfs.ext4 -F -L persistent <p5>`

### 2. RAID-1 Setup (`installer/internal/disks/mdadm.go`)

- Clears prior superblocks on both disks: `mdadm --zero-superblock --force <disk1>` and `mdadm --zero-superblock --force <disk2>`
- Creates RAID-1 mirror at block-device level: `mdadm --create /dev/md0 --level=1 --raid-devices=2 --metadata=1.2 --run <disk1> <disk2>`
- Partition layout is applied to `/dev/md0` (same as single-disk case)

### 3. GRUB Installation (`installer/internal/install/grub.go`)

- Installs GRUB for EFI: `grub-install --target=x86_64-efi --efi-directory=<efi_mount> --boot-directory=<boot_mount> --bootloader-id=novanas --removable`
- Writes `/boot/grub/grub.cfg` with minimal A/B boot configuration:
  - Sets `default=A` (Slot A boots by default)
  - Sets `timeout=3` (3-second timeout before default boot)
  - Loads GRUB environment from Boot partition via `load_env --file=/grubenv` (for RAUC BOOT_ORDER semantics)
  - Defines menuentry "NovaNas (Slot A)" with kernel args: `root=LABEL=os-a ro quiet rauc.slot=A`
  - Defines menuentry "NovaNas (Slot B)" with kernel args: `root=LABEL=os-b ro quiet rauc.slot=B`
  - Both entries search by filesystem label and load `/boot/vmlinuz` and `/boot/initrd.img`

### 4. Squashfs Extraction (`installer/internal/install/squashfs.go`)

- Probes for live-boot squashfs in order of preference:
  1. `/run/live/medium/live/filesystem.squashfs` (canonical live-boot media mountpoint)
  2. `/lib/live/mount/medium/live/filesystem.squashfs` (older live-boot)
  3. `/cdrom/live/filesystem.squashfs` (some bootloader paths)
  4. `/media/cdrom/live/filesystem.squashfs` (fallback)
- Extracts squashfs to target slot via: `unsquashfs -f -d <mountpoint> -no-progress <source>` (with `-f` to overwrite and `-no-progress` to suppress progress bar)
- Currently used to populate initial OS image to Slot A during installation

### 5. RAUC Bundle Verification & Extraction (`installer/internal/install/rauc.go`)

**Verification:**
- Sanity-checks bundle exists and is at least 1 MiB in size
- Reads RAUC keyring from `/etc/rauc/keyring.pem` (default) or custom path
- Attempts in-process PKCS#7 signature verification of classic RAUC bundle layout (squashfs + appended detached CMS signature with big-endian uint32 trailer)
- Falls back to shellout if in-process verification fails: `rauc verify --keyring=<keyring> <bundle>`

**Extraction:**
- Unpacks bundle rootfs to target mountpoint via: `unsquashfs -f -d <mountpoint> <bundle>`

### 6. Persistent Partition Seeding (`installer/internal/install/persistent.go`)

- Creates directory structure on persistent partition:
  - `etc/overlay`
  - `var/log`
  - `var/lib/novanas`
  - `var/lib/rancher/k3s`
  - `var/lib/postgresql`
  - `var/lib/openbao`
  - `home/novanas-admin`
  - `opt/novanas`
- Writes nmstate network YAML to `/etc/novanas/network.yaml` (populated from wizard input)
- Writes version manifest to `/etc/novanas/version` with `channel=<channel>\nversion=<version>\n`
- Writes installer completion marker to `/etc/novanas/installer-done` with content `ok\n`

### 7. Network Configuration Wizard (`installer/internal/wizard/network.go`)

- Enumerates NICs via `network.List()` and displays each with MAC address and link status
- Prompts user to select management NIC
- Presents DHCP vs Static IP mode choice
- Collects hostname (defaults to "novanas")
- If Static IP selected:
  - Collects Address in CIDR notation (e.g., "192.168.1.50/24")
  - Collects Gateway IP (e.g., "192.168.1.1")
  - Collects Primary DNS (e.g., "1.1.1.1")
  - Collects Secondary DNS optional (e.g., "1.0.0.1")
- Returns composite network config map with keys: `iface`, `hostname`, `dhcp`, `address`, `gateway`, `dns`

---

## Step 2: d-i Replacement Mapping

| Current Action | d-i Replacement |
|---|---|
| GPT partition table creation (`parted mklabel gpt`) | `partman-auto/expert_recipe` with `gpt` table type |
| EFI partition 512 MiB with ESP flag | `partman-auto/expert_recipe` partition 1: size 512, format vfat, flag boot |
| Boot partition 2 GiB ext4 | `partman-auto/expert_recipe` partition 2: size 2048, format ext4 |
| OS-A partition 4 GiB ext4 | `partman-auto/expert_recipe` partition 3: size 4096, format ext4 |
| OS-B partition 4 GiB ext4 | `partman-auto/expert_recipe` partition 4: size 4096, format ext4 |
| Persistent partition 80 GiB ext4 (remaining capacity up to 80 GiB, unallocated remainder) | `partman-auto/expert_recipe` partition 5: size -1 (consume to limit), format ext4 |
| Format filesystems (mkfs.vfat, mkfs.ext4) | Handled automatically by partman during recipe execution |
| RAID-1 mirror via `mdadm --create` | `partman-md` preseeding: `partman-md/device_remove_md0` (if present), then `partman-md/confirm_nochanges` false and `partman-md/confirm` true to create mirror |
| GRUB EFI install with x86_64-efi target | `grub-installer/bootloader_id=novanas`, `grub-installer/grub2_instead_of_grub_legacy=true`, `grub-installer/choose_bootloader=grub2` |
| GRUB config with A/B boot entries and RAUC integration | `late_command`: execute custom script to generate and write `/boot/grub/grub.cfg` with A/B logic and `load_env` directive |
| Extract squashfs to Slot A | `late_command`: `unsquashfs -f -d /target/os-a-mount <squashfs_source>` or RAUC install if initial is a RAUC bundle |
| RAUC signature verification (`rauc verify`) | `late_command`: `rauc verify --keyring=/target/etc/rauc/keyring.pem <bundle>` if using RAUC bundles for slot updates |
| Initial image deployment to Slot A | `late_command`: either squashfs extraction or `late_command`: `rauc install /cdrom/novanas-initial.raucb` if using RAUC bundle |
| RAUC configuration (keyring, system.conf) | `late_command`: write `/target/etc/rauc/system.conf` with slot definitions and RAUC config; ensure keyring at `/target/etc/rauc/keyring.pem` |
| Persistent partition directory structure and seeding | `late_command`: execute script to create directory tree and write nmstate YAML, version manifest, and installer-done marker |
| Network DHCP vs Static configuration | `netcfg/get_hostname=novanas` (default), `netcfg/choose_interface=<iface>`, `netcfg/confirm_static=true` (if static), `netcfg/get_ipaddress=<addr>`, `netcfg/get_netmask=<netmask>`, `netcfg/get_gateway=<gw>`, `netcfg/get_nameservers=<dns_list>` |
| Persistent mount configuration (fstab) | `late_command`: write `/target/etc/fstab` entries for persistent partition at `/persistent` or mount point determined by recipe |
| NIC enumeration and user selection | `netcfg` preseed keys (`netcfg/choose_interface`) allow pre-selection or preseed to skip wizard |
| Hostname configuration | `netcfg/get_hostname=<hostname>` preseed key |

---

## Notes

- **Squashfs source paths:** The current installer probes multiple live-boot media paths; the d-i migration must ensure the ISO build process places the squashfs at one of these paths or provide it via preseed variable.
- **RAUC integration:** Current installer supports both direct squashfs extraction and RAUC bundle verification/extraction. The d-i migration should clarify whether to use RAUC bundles or direct squashfs for initial image, then route late_command accordingly.
- **Persistent seeding:** Network YAML configuration is built from wizard input and seeded during installation. The d-i approach must capture wizard choices (DHCP vs static, DNS, gateway) and either preseed them directly or collect during d-i phases and write to persistent partition in `late_command`.
- **Partition sizes:** All sizes are hardcoded in `DefaultLayout()`. The d-i recipe should make these easily configurable via variables if needed for different target disk sizes.
