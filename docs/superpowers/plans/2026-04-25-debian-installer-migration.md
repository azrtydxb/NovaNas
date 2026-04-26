# Debian-Installer Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the in-house bubbletea TUI installer (`installer/`) with a `debian-installer`-based ISO that uses preseed + a custom partman recipe to lay down the RAUC A/B layout, then `rauc install`s the initial NovaNas bundle.

**Architecture:** The new installer ISO is built by repacking the official `debian-trixie-netinst.iso` with our preseed embedded in `initrd.gz`, our custom partman recipe shipped via `late_command`, and an initial RAUC bundle bundled on the ISO. d-i handles locale/keyboard/timezone/network/disk-layout (mature, polyglot, professional). After d-i finishes the base Debian install, `late_command` installs RAUC, runs `rauc install /cdrom/novanas-initial.raucb` to overlay the actual NovaNas image into slot A, copies it to slot B for redundancy, then reboots.

**Tech Stack:** debian-installer (preseed v3.x), partman + partman-md for RAID-1 GPT layout, RAUC for A/B slot management, xorriso for ISO repack, mkosi for the RAUC bundle (unchanged).

---

## File Structure

**New files:**
- `installer-di/preseed.cfg` — preseed answers for d-i (locale, keyboard, network, partman, late_command)
- `installer-di/partman-recipe.txt` — partman expert recipe defining RAID-1 + GPT layout
- `installer-di/late_command.sh` — runs after d-i finishes; installs RAUC and the initial bundle
- `installer-di/grub-theme/` — branded GRUB boot menu for the installer ISO
- `installer-di/build-installer-iso.sh` — repacks Debian netinst ISO with our customizations
- `os/build/build-iso.sh` — replaced (was mkosi-based, now d-i-based)
- `docs/06-partition-layout.md` — updated to reference partman recipe

**Modified files:**
- `os/Makefile` — `mkosi-live` target removed; `iso` target now calls `build-installer-iso.sh`
- `.github/workflows/os-build.yml` — installer Go build step removed; netinst download cached
- `os/mkosi.conf.d/30-live.conf` — removed (live profile no longer exists)
- `os/mkosi.extra-live/` — removed (no live ISO to extra-tree files into)
- `os/rootfs/etc/systemd/system/novanas-installer.service` — deleted
- `os/rootfs/etc/systemd/system/novanas-installer-watchdog.service` — deleted
- `os/rootfs/usr/local/bin/novanas-live-installer.sh` — deleted

**Deleted (after verification):**
- `installer/` — entire Go bubbletea installer codebase
- All references to `INSTALLER_BINARY` env var across CI / Makefile

---

## Task 0: Capture current install state and document target architecture

**Files:**
- Create: `docs/superpowers/plans/2026-04-25-current-installer-inventory.md`

- [ ] **Step 1: Inventory current installer actions**

Read each of these and write one bullet per action to the inventory doc:
- `installer/internal/disks/partition.go` (parted commands, sizes)
- `installer/internal/disks/mdadm.go` (mdadm RAID-1 setup)
- `installer/internal/install/grub.go` (grub-install args, RAUC-aware grub.cfg)
- `installer/internal/install/squashfs.go` (squashfs source paths, target slot)
- `installer/internal/install/rauc.go` (RAUC verify + extract logic)
- `installer/internal/install/persistent.go` (persistent mount setup)
- `installer/internal/wizard/network.go` (DHCP vs static, hostname)

- [ ] **Step 2: Define the post-install equivalence**

For each action above, write the d-i mechanism that replaces it:

| Current action | d-i replacement |
|---|---|
| Parted GPT layout | `partman-auto/expert_recipe` |
| RAID-1 mirror | `partman-md` |
| GRUB EFI install | `grub-installer` |
| Initial squashfs to slot A | `late_command`: `rauc install /cdrom/novanas-initial.raucb` |
| RAUC config | `late_command` writes `/etc/rauc/system.conf` |
| Persistent mount | partman recipe creates the FS; late_command writes /etc/fstab entry |
| Network config | `netcfg/get_*` preseed keys |

- [ ] **Step 3: Commit inventory**

```bash
git add docs/superpowers/plans/2026-04-25-current-installer-inventory.md
git commit -m "docs: inventory current installer actions for d-i migration"
```

---

## Task 1: Create the d-i build pipeline skeleton

**Files:**
- Create: `installer-di/build-installer-iso.sh`
- Create: `installer-di/.gitignore`

- [ ] **Step 1: Write the build script skeleton**

```bash
#!/usr/bin/env bash
# Build a NovaNas installer ISO by repacking debian-trixie-netinst.iso
# with our preseed + late_command + initial RAUC bundle embedded.

set -euo pipefail

NETINST_URL="${NETINST_URL:-https://cdimage.debian.org/cdimage/release/current/amd64/iso-cd/debian-13.0.0-amd64-netinst.iso}"
NETINST_SHA256="${NETINST_SHA256:-}"  # set by caller
OUT_ISO="${OUT_ISO:-build/out/novanas-installer.iso}"
RAUC_BUNDLE="${RAUC_BUNDLE:-build/out/novanas.raucb}"
WORK_DIR="${WORK_DIR:-build/installer-di-work}"

log() { printf '[build-installer-iso] %s\n' "$*" >&2; }

main() {
  command -v xorriso >/dev/null 2>&1 || { echo "xorriso not installed"; exit 1; }
  command -v gunzip  >/dev/null 2>&1 || { echo "gunzip not installed"; exit 1; }

  mkdir -p "$WORK_DIR" "$(dirname "$OUT_ISO")"

  log "step 1: download netinst"
  log "step 2: unpack ISO contents"
  log "step 3: inject preseed into initrd"
  log "step 4: copy late_command + RAUC bundle into ISO"
  log "step 5: customize grub menu"
  log "step 6: rebuild ISO with xorriso"

  # implementation in subsequent tasks
}

main "$@"
```

- [ ] **Step 2: Create gitignore for work dir**

```
build/
*.iso
netinst-cache/
```

- [ ] **Step 3: chmod +x and commit**

```bash
chmod +x installer-di/build-installer-iso.sh
git add installer-di/
git commit -m "feat(installer-di): scaffold d-i ISO build pipeline"
```

---

## Task 2: Implement netinst ISO download with checksum verification

**Files:**
- Modify: `installer-di/build-installer-iso.sh`
- Test: `installer-di/tests/test-download.sh`

- [ ] **Step 1: Write the test (verify download fails on bad SHA)**

```bash
#!/usr/bin/env bash
# tests/test-download.sh
set -e
cd "$(dirname "$0")/.."
NETINST_URL="https://example.invalid/nope.iso" \
  NETINST_SHA256="0000" \
  ./build-installer-iso.sh 2>&1 | grep -q "checksum mismatch" \
  && echo "PASS" || { echo "FAIL: expected checksum mismatch error"; exit 1; }
```

- [ ] **Step 2: Run the test (expect FAIL — function doesn't exist yet)**

```bash
bash installer-di/tests/test-download.sh
# Expected: FAIL with "checksum mismatch" not seen because function not implemented
```

- [ ] **Step 3: Implement download_netinst**

Add to `build-installer-iso.sh`, replacing the `step 1: download netinst` line:

```bash
download_netinst() {
  local cache="netinst-cache/$(basename "$NETINST_URL")"
  mkdir -p "$(dirname "$cache")"
  if [[ ! -f "$cache" ]]; then
    log "fetching $NETINST_URL"
    curl -fL --retry 3 -o "$cache.tmp" "$NETINST_URL"
    mv "$cache.tmp" "$cache"
  fi
  if [[ -n "$NETINST_SHA256" ]]; then
    local got
    got=$(sha256sum "$cache" | awk '{print $1}')
    if [[ "$got" != "$NETINST_SHA256" ]]; then
      log "checksum mismatch: expected $NETINST_SHA256, got $got"
      rm -f "$cache"
      exit 1
    fi
  fi
  echo "$cache"
}
```

- [ ] **Step 4: Run the test (expect PASS)**

```bash
bash installer-di/tests/test-download.sh
# Expected: PASS
```

- [ ] **Step 5: Commit**

```bash
git add installer-di/
git commit -m "feat(installer-di): netinst download with sha256 verification"
```

---

## Task 3: Implement ISO unpack and preseed injection

**Files:**
- Modify: `installer-di/build-installer-iso.sh`
- Create: `installer-di/preseed.cfg` (minimal stub for now)

- [ ] **Step 1: Write a minimal preseed.cfg stub**

```
# Minimal preseed for build pipeline test.
# Actual answers are filled in by Task 5.
d-i debian-installer/locale string en_US.UTF-8
d-i keyboard-configuration/xkb-keymap select us
```

- [ ] **Step 2: Implement unpack_iso**

Add to `build-installer-iso.sh`:

```bash
unpack_iso() {
  local iso="$1"
  local dest="$WORK_DIR/iso"
  rm -rf "$dest"
  mkdir -p "$dest"
  log "extracting $iso to $dest"
  xorriso -osirrox on -indev "$iso" -extract / "$dest"
  chmod -R u+w "$dest"
}
```

- [ ] **Step 3: Implement inject_preseed (rebuild initrd with preseed)**

```bash
inject_preseed() {
  local initrd="$WORK_DIR/iso/install.amd/initrd.gz"
  local stage="$WORK_DIR/initrd-stage"
  rm -rf "$stage"
  mkdir -p "$stage"
  log "unpacking initrd"
  ( cd "$stage" && gunzip < "../../$initrd" | cpio -id --quiet )
  log "copying preseed into initrd"
  cp installer-di/preseed.cfg "$stage/preseed.cfg"
  log "repacking initrd"
  ( cd "$stage" && find . | cpio --create --format=newc --quiet | gzip > "../../$initrd" )
}
```

- [ ] **Step 4: Wire into main()**

Replace the placeholders `step 1`-`step 3` with real calls:

```bash
ISO_PATH=$(download_netinst)
unpack_iso "$ISO_PATH"
inject_preseed
```

- [ ] **Step 5: Test build runs to step 4**

```bash
NETINST_URL="https://cdimage.debian.org/cdimage/release/current/amd64/iso-cd/debian-13.0.0-amd64-netinst.iso" \
  installer-di/build-installer-iso.sh
# Expected: completes through step 5 (grub customize), then exits without producing ISO yet (xorriso step still placeholder)
```

- [ ] **Step 6: Commit**

```bash
git add installer-di/build-installer-iso.sh installer-di/preseed.cfg
git commit -m "feat(installer-di): unpack netinst + inject preseed into initrd"
```

---

## Task 4: Implement grub menu customization for unattended install

**Files:**
- Modify: `installer-di/build-installer-iso.sh`
- Create: `installer-di/grub-installer.cfg`

- [ ] **Step 1: Write the GRUB menu we want**

```
# installer-di/grub-installer.cfg
set timeout=5
set default=0

menuentry "Install NovaNas (automatic)" {
    linux  /install.amd/vmlinuz auto=true priority=critical preseed/file=/preseed.cfg quiet ---
    initrd /install.amd/initrd.gz
}

menuentry "Install NovaNas (interactive)" {
    linux  /install.amd/vmlinuz preseed/file=/preseed.cfg ---
    initrd /install.amd/initrd.gz
}

menuentry "Rescue mode" {
    linux  /install.amd/vmlinuz rescue/enable=true ---
    initrd /install.amd/initrd.gz
}
```

`auto=true priority=critical` makes d-i ask only critical questions (which the preseed will answer all of), so install runs unattended. Interactive mode keeps the preseed but presents each screen so the operator can override.

- [ ] **Step 2: Implement customize_grub**

```bash
customize_grub() {
  log "writing GRUB menu"
  cp installer-di/grub-installer.cfg "$WORK_DIR/iso/boot/grub/grub.cfg"
  # Also overwrite isolinux for legacy boot
  if [[ -f "$WORK_DIR/iso/isolinux/isolinux.cfg" ]]; then
    cat > "$WORK_DIR/iso/isolinux/isolinux.cfg" <<'EOF'
default novanas-auto
prompt 0
timeout 50

label novanas-auto
  menu label Install NovaNas (automatic)
  kernel /install.amd/vmlinuz
  append auto=true priority=critical preseed/file=/preseed.cfg vga=788 initrd=/install.amd/initrd.gz quiet ---

label novanas-interactive
  menu label Install NovaNas (interactive)
  kernel /install.amd/vmlinuz
  append preseed/file=/preseed.cfg vga=788 initrd=/install.amd/initrd.gz ---
EOF
  fi
}
```

- [ ] **Step 3: Wire into main()**

Replace `step 5: customize grub menu` placeholder with `customize_grub`.

- [ ] **Step 4: Commit**

```bash
git add installer-di/
git commit -m "feat(installer-di): GRUB + isolinux menus default to autoinstall"
```

---

## Task 5: Implement xorriso ISO rebuild (hybrid BIOS+EFI bootable)

**Files:**
- Modify: `installer-di/build-installer-iso.sh`

- [ ] **Step 1: Implement rebuild_iso**

```bash
rebuild_iso() {
  log "repacking ISO -> $OUT_ISO"
  xorriso \
    -as mkisofs \
    -r -V "NovaNas Installer" \
    -o "$OUT_ISO" \
    -isohybrid-mbr "$WORK_DIR/iso/isolinux/isohdpfx.bin" \
    -c isolinux/boot.cat \
    -b isolinux/isolinux.bin \
    -no-emul-boot -boot-load-size 4 -boot-info-table \
    -eltorito-alt-boot \
    -e boot/grub/efi.img \
    -no-emul-boot \
    -isohybrid-gpt-basdat \
    "$WORK_DIR/iso"
}
```

- [ ] **Step 2: Wire into main(), removing the placeholder**

```bash
rebuild_iso
log "DONE: $OUT_ISO ($(stat -c %s "$OUT_ISO" 2>/dev/null || stat -f %z "$OUT_ISO") bytes)"
```

- [ ] **Step 3: Run end-to-end build locally**

```bash
installer-di/build-installer-iso.sh
ls -lh build/out/novanas-installer.iso
```

Expected: an ISO file ~700MB (netinst + customizations).

- [ ] **Step 4: Boot the ISO in QEMU to verify it boots to d-i**

```bash
qemu-system-x86_64 -m 2G -enable-kvm \
  -drive file=build/out/novanas-installer.iso,media=cdrom \
  -drive file=$(mktemp /tmp/disk1.XXXXXX),size=10G,if=virtio \
  -drive file=$(mktemp /tmp/disk2.XXXXXX),size=10G,if=virtio \
  -boot d
```

Expected: boots to GRUB menu showing "Install NovaNas (automatic)". After 5s autoboots and d-i starts.

- [ ] **Step 5: Commit**

```bash
git add installer-di/build-installer-iso.sh
git commit -m "feat(installer-di): hybrid BIOS+EFI bootable ISO via xorriso"
```

---

## Task 6: Write the full preseed.cfg

**Files:**
- Modify: `installer-di/preseed.cfg`

- [ ] **Step 1: Replace stub with full preseed**

```
# NovaNas installer preseed for debian-trixie.
# Reference: https://www.debian.org/releases/trixie/amd64/apbs04.en.html

### Localization
d-i debian-installer/locale string en_US.UTF-8
d-i debian-installer/language string en
d-i debian-installer/country string US
d-i keyboard-configuration/xkb-keymap select us

### Network
# Hostname is set by netcfg from DHCP if available; otherwise this default.
d-i netcfg/choose_interface select auto
d-i netcfg/get_hostname string novanas
d-i netcfg/get_domain string local
d-i netcfg/hostname string novanas
d-i netcfg/wireless_wep string

### Mirror
d-i mirror/country string manual
d-i mirror/http/hostname string deb.debian.org
d-i mirror/http/directory string /debian
d-i mirror/http/proxy string

### Time
d-i clock-setup/utc boolean true
d-i time/zone string Etc/UTC
d-i clock-setup/ntp boolean true

### Account setup
d-i passwd/root-login boolean false
d-i passwd/make-user boolean true
d-i passwd/user-fullname string NovaNas Operator
d-i passwd/username string novanas
# Password is set interactively (passwd-prompt mode) — never hardcode.
d-i passwd/user-password-crypted password !
d-i user-setup/allow-password-weak boolean false
d-i user-setup/encrypt-home boolean false

### Partitioning — driven by partman-auto/expert_recipe (defined in next task)
d-i partman-auto/method string raid
d-i partman-auto/disk string /dev/nvme0n1 /dev/nvme1n1
d-i partman-auto/choose_recipe select novanas
d-i partman-md/device_remove_md boolean true
d-i partman-md/confirm boolean true
d-i partman-partitioning/confirm_write_new_label boolean true
d-i partman/choose_partition select finish
d-i partman/confirm boolean true
d-i partman/confirm_nooverwrite boolean true

### Apt setup
d-i apt-setup/non-free-firmware boolean true
d-i apt-setup/non-free boolean false
d-i apt-setup/contrib boolean false

### Package selection — minimal base; everything else comes from RAUC bundle
tasksel tasksel/first multiselect standard
d-i pkgsel/include string openssh-server rauc curl ca-certificates parted dosfstools mdadm
d-i pkgsel/upgrade select none
popularity-contest popularity-contest/participate boolean false

### Bootloader
d-i grub-installer/only_debian boolean true
d-i grub-installer/with_other_os boolean false
d-i grub-installer/bootdev string /dev/nvme0n1 /dev/nvme1n1

### Finish
d-i finish-install/reboot_in_progress note
d-i debian-installer/exit/reboot boolean true

### late_command — runs in /target chroot after install
d-i preseed/late_command string \
  cp /cdrom/novanas/late_command.sh /target/tmp/late_command.sh; \
  cp /cdrom/novanas/novanas-initial.raucb /target/var/cache/novanas-initial.raucb; \
  in-target /bin/bash /tmp/late_command.sh
```

- [ ] **Step 2: Run linter on preseed file (debconf-set-selections check)**

```bash
debconf-set-selections --checkonly < installer-di/preseed.cfg
echo "Exit: $?"
```

Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add installer-di/preseed.cfg
git commit -m "feat(installer-di): full preseed for locale/network/account/apt/grub"
```

---

## Task 7: Write the partman expert recipe for RAID-1 + RAUC A/B layout

**Files:**
- Create: `installer-di/partman-recipe.txt`
- Modify: `installer-di/preseed.cfg` (inline the recipe via `d-i partman-auto/expert_recipe string \`)

- [ ] **Step 1: Write the partman recipe**

`installer-di/partman-recipe.txt`:

```
novanas ::                                                              \
        1 1 1 free                                                      \
                $bios_boot{ }                                           \
                method{ biosgrub } .                                    \
        538 538 1075 raid                                               \
                $primary{ } $bootable{ }                                \
                method{ raid } .                                        \
        1024 1024 2048 raid                                             \
                method{ raid } .                                        \
        4096 4096 4096 raid                                             \
                method{ raid } .                                        \
        4096 4096 4096 raid                                             \
                method{ raid } .                                        \
        20480 1000000000 1000000000 raid                                \
                method{ raid } .
```

This is per-disk; partman replicates it on `/dev/nvme0n1` AND `/dev/nvme1n1`, then `partman-md` mirrors them into RAID-1 arrays:
- md0 = ESP mirror (538MB FAT32 → /boot/efi)
- md1 = /boot mirror (1GB ext4)
- md2 = OS-A slot (4GB ext4)
- md3 = OS-B slot (4GB ext4)
- md4 = Persistent storage (rest of disk, ext4 → /var/lib/novanas)

- [ ] **Step 2: Define the RAID arrays via preseed (append to preseed.cfg)**

```
### RAID array definitions
# Format: <type> <devcount> <sparecount> <fstype> <mountpoint> <devices> <spares>
d-i partman-auto-raid/recipe string \
  1 2 0 vfat /boot/efi      /dev/nvme0n1p2#/dev/nvme1n1p2 .   \
  1 2 0 ext4 /boot          /dev/nvme0n1p3#/dev/nvme1n1p3 .   \
  1 2 0 ext4 /              /dev/nvme0n1p4#/dev/nvme1n1p4 .   \
  1 2 0 ext4 /mnt/os-b      /dev/nvme0n1p5#/dev/nvme1n1p5 .   \
  1 2 0 ext4 /var/lib/novanas /dev/nvme0n1p6#/dev/nvme1n1p6 .
```

NOTE: `/` mounts on md2 (OS-A); md3 (OS-B) mounts on `/mnt/os-b` so late_command can rsync into it. After install RAUC will manage which one is "current" via slot status — for the first boot it's always slot A.

- [ ] **Step 3: Embed recipe into preseed**

Convert recipe to single-line preseed-friendly format. Add to `preseed.cfg`:

```
d-i partman-auto/expert_recipe string \
  novanas :: \
    1 1 1 free $bios_boot{ } method{ biosgrub } . \
    538 538 1075 raid $primary{ } $bootable{ } method{ raid } . \
    1024 1024 2048 raid method{ raid } . \
    4096 4096 4096 raid method{ raid } . \
    4096 4096 4096 raid method{ raid } . \
    20480 1000000000 1000000000 raid method{ raid } .
```

- [ ] **Step 4: Validate partition recipe syntax with d-i in QEMU**

```bash
installer-di/build-installer-iso.sh
qemu-system-x86_64 -m 2G -enable-kvm \
  -drive file=build/out/novanas-installer.iso,media=cdrom \
  -drive file=/tmp/disk1.img,size=20G,if=virtio,format=raw \
  -drive file=/tmp/disk2.img,size=20G,if=virtio,format=raw \
  -boot d
```

Expected: d-i runs through preseed, partitions both disks, creates RAID-1 arrays, formats, installs Debian to /, finishes without manual prompts.

- [ ] **Step 5: Commit**

```bash
git add installer-di/
git commit -m "feat(installer-di): partman recipe for RAID-1 RAUC A/B layout"
```

---

## Task 8: Write late_command.sh — install RAUC + initial bundle

**Files:**
- Create: `installer-di/late_command.sh`
- Modify: `installer-di/build-installer-iso.sh` (copy script + initial RAUC bundle into ISO)

- [ ] **Step 1: Write the late_command script**

```bash
#!/bin/bash
# Runs inside /target chroot after d-i finishes the base install.
# Responsibilities:
#  1. Install RAUC (already in pkgsel/include) and configure /etc/rauc/system.conf
#  2. Configure GRUB to use RAUC's A/B boot ordering
#  3. Run `rauc install` to overlay the actual NovaNas bundle into slot A
#  4. Mark slot A as good

set -euo pipefail
log() { printf '[late_command] %s\n' "$*" >&2; }

log "writing /etc/rauc/system.conf"
mkdir -p /etc/rauc
cat > /etc/rauc/system.conf <<'EOF'
[system]
compatible=novanas-amd64
bootloader=grub
mountprefix=/run/rauc

[keyring]
path=/etc/rauc/keyring.pem

[slot.rootfs.0]
device=/dev/md/2
type=ext4
bootname=A

[slot.rootfs.1]
device=/dev/md/3
type=ext4
bootname=B

[slot.bootloader.0]
device=/dev/md/1
type=ext4
EOF

log "installing RAUC keyring"
mkdir -p /etc/rauc
cp /var/cache/novanas-keyring.pem /etc/rauc/keyring.pem 2>/dev/null \
  || log "WARN: no keyring shipped; first 'rauc install' will fail signature check"

log "writing GRUB RAUC-aware menu fragment"
cat > /etc/default/grub.d/90-rauc.cfg <<'EOF'
GRUB_DISTRIBUTOR="NovaNas"
GRUB_CMDLINE_LINUX_DEFAULT="quiet rauc.slot=A"
GRUB_DISABLE_OS_PROBER=true
EOF
update-grub

log "writing /etc/grub.d/30_rauc"
cat > /etc/grub.d/30_rauc <<'EOF'
#!/bin/sh
exec tail -n +3 \$0
menuentry 'NovaNas (slot A)' { linux /vmlinuz root=/dev/md/2 ro rauc.slot=A; initrd /initrd.img }
menuentry 'NovaNas (slot B)' { linux /vmlinuz root=/dev/md/3 ro rauc.slot=B; initrd /initrd.img }
EOF
chmod +x /etc/grub.d/30_rauc
update-grub

log "writing /etc/fstab persistent mount"
echo "/dev/md/4  /var/lib/novanas  ext4  defaults  0 2" >> /etc/fstab

if [[ -f /var/cache/novanas-initial.raucb ]]; then
  log "running rauc install of initial bundle"
  rauc install /var/cache/novanas-initial.raucb || log "WARN: rauc install failed; system will boot stock Debian on first boot"
  rauc status mark-good booted || true
  rm -f /var/cache/novanas-initial.raucb
else
  log "no initial RAUC bundle present; system will boot stock Debian"
fi

log "late_command done"
```

- [ ] **Step 2: Wire into build-installer-iso.sh — copy script + bundle**

Add a `copy_payload` function:

```bash
copy_payload() {
  local payload_dir="$WORK_DIR/iso/novanas"
  mkdir -p "$payload_dir"
  cp installer-di/late_command.sh "$payload_dir/"
  if [[ -f "$RAUC_BUNDLE" ]]; then
    log "copying RAUC bundle ($(stat -c %s "$RAUC_BUNDLE") bytes)"
    cp "$RAUC_BUNDLE" "$payload_dir/novanas-initial.raucb"
  else
    log "WARN: no RAUC bundle at $RAUC_BUNDLE; ISO will install stock Debian"
  fi
  if [[ -f os/rauc/keyring.pem ]]; then
    cp os/rauc/keyring.pem "$payload_dir/novanas-keyring.pem"
  fi
}
```

Call `copy_payload` after `inject_preseed`.

- [ ] **Step 3: Commit**

```bash
git add installer-di/
git commit -m "feat(installer-di): late_command installs RAUC + initial bundle"
```

---

## Task 9: End-to-end QEMU verification

**Files:**
- Create: `installer-di/tests/e2e-qemu.sh`

- [ ] **Step 1: Write the e2e test**

```bash
#!/usr/bin/env bash
# Boot the installer ISO in QEMU with two virtual disks; verify install
# completes without prompts and the resulting system boots successfully.
set -euo pipefail

ISO="${1:-build/out/novanas-installer.iso}"
DISK1=$(mktemp /tmp/novanas-disk1.XXXXXX.img)
DISK2=$(mktemp /tmp/novanas-disk2.XXXXXX.img)
trap 'rm -f "$DISK1" "$DISK2"' EXIT
qemu-img create -f raw "$DISK1" 20G
qemu-img create -f raw "$DISK2" 20G

# Phase 1: install
qemu-system-x86_64 \
  -m 2G -enable-kvm \
  -drive "file=$ISO,media=cdrom" \
  -drive "file=$DISK1,if=virtio,format=raw" \
  -drive "file=$DISK2,if=virtio,format=raw" \
  -boot d \
  -nographic \
  -serial mon:stdio \
  -no-reboot \
  -append "auto=true priority=critical preseed/file=/preseed.cfg console=ttyS0,115200n8" \
  | tee install.log

grep -q "late_command done" install.log || {
  echo "FAIL: install did not complete cleanly"
  exit 1
}

# Phase 2: boot the installed system
qemu-system-x86_64 \
  -m 2G -enable-kvm \
  -drive "file=$DISK1,if=virtio,format=raw" \
  -drive "file=$DISK2,if=virtio,format=raw" \
  -nographic -serial mon:stdio \
  -no-reboot \
  | tee boot.log &
QEMU_PID=$!
sleep 60
kill "$QEMU_PID" 2>/dev/null || true

grep -q "novanas login:" boot.log || {
  echo "FAIL: installed system did not reach login prompt"
  exit 1
}

echo "PASS: install + boot succeeded"
```

- [ ] **Step 2: Run e2e test**

```bash
bash installer-di/tests/e2e-qemu.sh
```

Expected: PASS in ~10 minutes.

- [ ] **Step 3: Boot the ISO on JetKVM (real hardware)**

Upload to rolling-dev release, mount via JetKVM virtual media, boot, verify:
- d-i menu appears (timestamp boot to track speed)
- Auto-install completes within ~15 minutes
- Reboots and shows `novanas login:` prompt

- [ ] **Step 4: Commit test**

```bash
git add installer-di/tests/
git commit -m "test(installer-di): e2e QEMU install + boot verification"
```

---

## Task 10: Replace os/build/build-iso.sh and update CI

**Files:**
- Modify: `os/build/build-iso.sh` (delete current contents, replace with thin wrapper)
- Modify: `os/Makefile` (mkosi-live target removed, iso target points to installer-di)
- Modify: `.github/workflows/os-build.yml` (remove installer Go build step)

- [ ] **Step 1: Rewrite os/build/build-iso.sh as a wrapper**

```bash
#!/usr/bin/env bash
# Thin wrapper that invokes the d-i-based ISO builder. The old mkosi-live
# pipeline was removed in favor of a debian-installer ISO repack — see
# installer-di/build-installer-iso.sh for the actual logic.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
exec "$REPO_ROOT/installer-di/build-installer-iso.sh" "$@"
```

- [ ] **Step 2: Strip mkosi-live target from os/Makefile**

Delete the `mkosi-live`, `live-artifacts`, `iso` (old), `iso-only` targets. Replace with:

```makefile
iso: bundle
	OUT_ISO=$(ISO) RAUC_BUNDLE=$(BUNDLE) $(CURDIR)/build/build-iso.sh

iso-only: | $(BUILD_DIR)
	@test -f $(BUNDLE) || { echo "BUNDLE missing: $(BUNDLE)" >&2; exit 1; }
	OUT_ISO=$(ISO) RAUC_BUNDLE=$(BUNDLE) $(CURDIR)/build/build-iso.sh
```

- [ ] **Step 3: Strip installer Go build step from CI**

In `.github/workflows/os-build.yml`, remove these steps:
- `actions/setup-go@v6` step
- `Build installer binary (baked into live rootfs)` step
- `Build live (ISO) rootfs via mkosi`
- `make -C os live-artifacts` (folded into update-artifacts only)

Keep:
- mkosi-update (RAUC bundle still built via mkosi)
- bundle target
- iso target (now d-i based)

- [ ] **Step 4: Commit**

```bash
git add os/ .github/
git commit -m "refactor(os): build-iso.sh now delegates to d-i installer pipeline"
```

---

## Task 11: Delete the old bubbletea installer

**Files:**
- Delete: `installer/` (entire directory)
- Delete: `os/mkosi.conf.d/30-live.conf`
- Delete: `os/mkosi.extra-live/`
- Delete: `os/rootfs/etc/systemd/system/novanas-installer.service`
- Delete: `os/rootfs/etc/systemd/system/novanas-installer-watchdog.service`
- Delete: `os/rootfs/usr/local/bin/novanas-live-installer.sh`

- [ ] **Step 1: Verify nothing else references the installer**

```bash
grep -rln 'installer/' --include='*.go' --include='*.yml' --include='*.sh' --include='Makefile' . | grep -v 'installer-di/' | grep -v 'docs/'
grep -rln 'novanas-installer.service\|novanas-installer-watchdog' . | grep -v docs/ | grep -v 'plans/'
```

Expected: empty output (or only files we're about to delete).

- [ ] **Step 2: Delete the directories and files**

```bash
git rm -rf installer/
git rm -rf os/mkosi.extra-live/
git rm -f os/mkosi.conf.d/30-live.conf
git rm -f os/rootfs/etc/systemd/system/novanas-installer.service
git rm -f os/rootfs/etc/systemd/system/novanas-installer-watchdog.service
git rm -f os/rootfs/usr/local/bin/novanas-live-installer.sh
```

- [ ] **Step 3: Verify build still works**

```bash
make -C os iso
ls -lh os/build/out/novanas-installer.iso
```

Expected: ISO builds successfully.

- [ ] **Step 4: Commit**

```bash
git commit -m "refactor: remove bubbletea installer (replaced by debian-installer)"
```

---

## Task 12: Update docs

**Files:**
- Modify: `docs/06-partition-layout.md`
- Create: `docs/15-installer.md`
- Modify: `README.md` (if installer is mentioned)

- [ ] **Step 1: Document the new installer architecture**

`docs/15-installer.md`:

```markdown
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
```

- [ ] **Step 2: Update partition layout doc**

Update `docs/06-partition-layout.md` to reference the partman recipe instead of the Go disks/partition.go.

- [ ] **Step 3: Commit docs**

```bash
git add docs/
git commit -m "docs: document d-i-based installer architecture"
```

---

## Verification Matrix

After all tasks complete, the following must hold:

| Property | How to verify |
|---|---|
| ISO boots on JetKVM | rolling-dev publish + virtual-media boot |
| ISO boots in QEMU | `installer-di/tests/e2e-qemu.sh` |
| Install completes unattended | `late_command done` line in serial log |
| Installed system boots | `novanas login:` prompt within 60s of post-install reboot |
| RAUC slot A is "good" | `rauc status` on installed system shows A=good, B=bad |
| Subsequent `rauc install` works | install a second bundle, verify slot B becomes "current" |
| Interactive mode works | boot with second menu entry, click through screens manually |
| Build is reproducible | two consecutive `make iso` produce same SHA256 (modulo timestamps) |
| CI green | os-build workflow passes on the migration commit |

---

## Out of Scope (future work)

- Branded GRUB theme / splash (Task 4 ships plain). Add later via `installer-di/grub-theme/`.
- Localized installer text. Default English; future task adds locales.
- Network-only install path (no RAUC bundle on ISO). Currently we always ship the bundle.
- Operator-supplied custom partman recipes. Out of scope for v1.
