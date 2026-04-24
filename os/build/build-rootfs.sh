#!/usr/bin/env bash
# Build the NovaNas Debian rootfs.
#
# Stage 1 (--stage=base): mmdebstrap into a tarball. Fast, cacheable, no
#   NovaNas bits yet. Output: --out=<base.tar>.
# Stage 2 (--stage=layered): extract base tar into a work dir, install
#   NovaNas layer (k3s + packages + rootfs skeleton + pre-pulled images),
#   pack to an ext4 image. Output: --out=<rootfs.img> + --boot-out=<boot.img>.
#
# Requires: mmdebstrap, tar, chroot, mksquashfs OR mke2fs + resize2fs.
# Must run as root or inside `fakechroot mmdebstrap` for stage 1; stage 2
# always needs real root for chroot + loop mounts.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OS_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

STAGE=""
VERSION=""
CHANNEL=""
ARCH="amd64"
IN=""
OUT=""
BOOT_OUT=""

usage() {
  cat <<EOF
Usage: $(basename "$0") --stage=base|layered --version=X --channel=Y [options]

Common:
  --version=<ver>     required
  --channel=<chan>    required (dev|edge|beta|stable|lts)
  --arch=<arch>       default: amd64

Stage base:
  --out=<tarball>     output base rootfs tarball

Stage layered:
  --in=<tarball>      input base tarball (from stage base)
  --out=<image>       output ext4 rootfs image
  --boot-out=<image>  output boot partition image (kernel/initrd/GRUB env)
EOF
}

for arg in "$@"; do
  case "$arg" in
    --stage=*)     STAGE="${arg#*=}" ;;
    --version=*)   VERSION="${arg#*=}" ;;
    --channel=*)   CHANNEL="${arg#*=}" ;;
    --arch=*)      ARCH="${arg#*=}" ;;
    --in=*)        IN="${arg#*=}" ;;
    --out=*)       OUT="${arg#*=}" ;;
    --boot-out=*)  BOOT_OUT="${arg#*=}" ;;
    -h|--help)     usage; exit 0 ;;
    *) echo "unknown arg: $arg" >&2; usage >&2; exit 2 ;;
  esac
done

[[ -n "$STAGE"   ]] || { echo "--stage required"   >&2; exit 2; }
[[ -n "$VERSION" ]] || { echo "--version required" >&2; exit 2; }
[[ -n "$CHANNEL" ]] || { echo "--channel required" >&2; exit 2; }
[[ -n "$OUT"     ]] || { echo "--out required"     >&2; exit 2; }

# shellcheck disable=SC1090
source "$OS_DIR/mmdebstrap.conf"

need_root() {
  if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    echo "this stage requires root (use sudo or run inside a privileged container)" >&2
    exit 1
  fi
}

log() { printf '[build-rootfs:%s] %s\n' "$STAGE" "$*"; }

build_base() {
  local need_tools=(mmdebstrap tar)
  for t in "${need_tools[@]}"; do
    command -v "$t" >/dev/null 2>&1 || { echo "missing tool: $t" >&2; exit 1; }
  done

  log "mmdebstrap $SUITE -> $OUT"
  mkdir -p "$(dirname "$OUT")"

  mmdebstrap \
    --variant="$VARIANT" \
    --architectures="$ARCH" \
    --components="$(echo "$COMPONENTS" | tr ' ' ',')" \
    --include="$INCLUDE" \
    "$SUITE" \
    "$OUT" \
    "$MIRROR"

  log "stamping version/channel"
  local tmp
  tmp=$(mktemp -d)
  tar -xf "$OUT" -C "$tmp"
  mkdir -p "$tmp/etc/novanas"
  printf '%s\n' "$VERSION" > "$tmp/etc/novanas/version"
  printf '%s\n' "$CHANNEL" > "$tmp/etc/novanas/channel"
  tar -cf "$OUT" -C "$tmp" .
  rm -rf "$tmp"
  log "base rootfs ready at $OUT"
}

build_layered() {
  need_root
  [[ -n "$IN" ]]       || { echo "--in required for layered" >&2; exit 2; }
  [[ -n "$BOOT_OUT" ]] || { echo "--boot-out required for layered" >&2; exit 2; }
  [[ -f "$IN" ]]       || { echo "base tarball not found: $IN" >&2; exit 1; }

  local work
  work=$(mktemp -d)
  trap 'umount -R "${work:-}"/dev/pts "${work:-}"/dev "${work:-}"/proc "${work:-}"/sys 2>/dev/null || true; rm -rf "${work:-}"' EXIT

  log "extract base -> $work"
  tar -xpf "$IN" -C "$work"

  log "overlay NovaNas rootfs skeleton"
  cp -a "$OS_DIR/rootfs/." "$work/"

  # grub-pc-bin postinst invokes grub-mkconfig, which needs /boot/grub.
  # Without this directory the install fails before our sources.list
  # even has a chance to be queried.
  mkdir -p "$work/boot/grub"

  log "prep chroot bind mounts"
  mount --bind /dev  "$work/dev"
  mount --bind /dev/pts "$work/dev/pts"
  mount --bind /proc "$work/proc"
  mount --bind /sys  "$work/sys"

  log "apt install layered packages"
  # Force a complete sources.list covering main + contrib + non-free +
  # non-free-firmware + security + updates. mmdebstrap's generated file
  # historically omits some components depending on its own version, so
  # we overwrite it before running apt-get update.
  cat > "$work/etc/apt/sources.list" <<EOF
deb $MIRROR $SUITE main contrib non-free non-free-firmware
deb $MIRROR $SUITE-updates main contrib non-free non-free-firmware
deb $SECURITY_MIRROR $SUITE-security main contrib non-free non-free-firmware
EOF
  # Remove the DEB822 file mmdebstrap may have written so the classic
  # list above is authoritative.
  rm -f "$work/etc/apt/sources.list.d/debian.sources"

  # Pre-seed locales so glibc's update-locale step doesn't fail with
  # "invalid locale settings" during apt postinst scripts.
  sed -i 's/^# *en_US.UTF-8/en_US.UTF-8/' "$work/etc/locale.gen" 2>/dev/null || \
    echo "en_US.UTF-8 UTF-8" >> "$work/etc/locale.gen"

  # Pre-seed grub-pc-bin so its postinst does not prompt for install
  # devices or try to write to a disk that doesn't exist in the chroot.
  cat > "$work/tmp/grub-pc.seed" <<'EOF'
grub-pc grub-pc/install_devices multiselect
grub-pc grub-pc/install_devices_empty boolean true
EOF

  chroot "$work" /bin/bash -eu -o pipefail -c "
    export DEBIAN_FRONTEND=noninteractive
    debconf-set-selections /tmp/grub-pc.seed
    locale-gen en_US.UTF-8
    update-locale LANG=en_US.UTF-8
    apt-get update
    apt-get install -y --no-install-recommends $LAYERED_PACKAGES
    apt-get clean
    rm -rf /var/lib/apt/lists/* /tmp/grub-pc.seed
  "

  # Install the compiled installer binary into the rootfs so the live-booted
  # system has it on PATH without mounting the ISO. CI ensures this artifact
  # exists before invoking 'make layered'; if it's missing we warn and
  # continue so local dev builds still work.
  local installer_bin="${INSTALLER_BINARY:-$OS_DIR/../installer/bin/novanas-installer}"
  if [[ -f "$installer_bin" ]]; then
    log "install novanas-installer -> /usr/local/bin"
    install -m 0755 "$installer_bin" "$work/usr/local/bin/novanas-installer"
  else
    log "WARN: installer binary not found at $installer_bin (live ISO autoinstall will fail)"
  fi

  log "regenerate initramfs (pick up live-boot hooks)"
  chroot "$work" /bin/bash -eu -o pipefail -c "
    export DEBIAN_FRONTEND=noninteractive
    update-initramfs -u -k all
  "

  log "install k3s $K3S_VERSION"
  chroot "$work" /bin/bash -eu -o pipefail -c "
    curl -sfL https://get.k3s.io -o /tmp/k3s-install.sh
    INSTALL_K3S_VERSION='$K3S_VERSION' \
      INSTALL_K3S_SKIP_START=true \
      INSTALL_K3S_SKIP_ENABLE=true \
      sh /tmp/k3s-install.sh
    rm -f /tmp/k3s-install.sh
    mkdir -p /var/lib/rancher/k3s/agent/images /opt/novanas
  "

  log "pre-pull container images"
  "$OS_DIR/build/prepull-images.sh" \
    "$work/var/lib/rancher/k3s/agent/images" \
    "$OS_DIR/build/image-manifest.txt" || log "prepull skipped (see script output)"

  log "enable NovaNas units + users"
  chroot "$work" /bin/bash -eu -o pipefail -c "
    useradd -m -s /bin/bash -u $DEFAULT_UID $DEFAULT_USER || true
    passwd -l $DEFAULT_USER || true
    usermod -aG sudo $DEFAULT_USER
    systemctl disable ssh 2>/dev/null || true
    systemctl enable systemd-networkd systemd-resolved systemd-timesyncd
    systemctl enable novanas-persistent.mount novanas-overlay-etc.mount novanas-overlay-var.mount 2>/dev/null || true
    systemctl enable novanas-firstboot.service novanas-healthcheck.service 2>/dev/null || true
    systemctl enable novanas-installer.service 2>/dev/null || true
    systemctl enable novanas-installer-watchdog.service 2>/dev/null || true
    locale-gen $LOCALE
    update-locale LANG=$LOCALE
    ln -sf /usr/share/zoneinfo/$TIMEZONE /etc/localtime
    printf '%s\n' '$VERSION' > /etc/novanas/version
    printf '%s\n' '$CHANNEL' > /etc/novanas/channel
  "

  log "unmount chroot binds"
  umount -R "$work"/dev "$work"/proc "$work"/sys

  # Build filesystem.squashfs for the installer ISO's live-boot layer from
  # the same rootfs contents we pack into ext4 below. We do it here (rather
  # than in build-iso.sh) because the chroot is already assembled and we
  # don't want to re-mount the ext4 image. Excludes: /boot (kernel+initrd
  # come via the ISO's /boot/ directly), /proc, /sys, /dev, /run.
  local squash_out
  squash_out="$(dirname "$OUT")/filesystem.squashfs"
  if command -v mksquashfs >/dev/null 2>&1; then
    log "pack rootfs squashfs -> $squash_out"
    rm -f "$squash_out"
    # Keep proc/ sys/ dev/ run/ tmp/ as empty directories so live-boot can
    # bind-mount the host versions onto them post switch_root. Excluding
    # them entirely (as `-e proc sys dev run tmp` did previously) made
    # /init panic with "mount point does not exist" during live boot.
    # Only exclude dirs that carry real content we don't want shipped:
    # /boot (kernel/initrd live on the ISO, not in squashfs) and the
    # apt cache.
    mksquashfs "$work" "$squash_out" \
      -noappend -comp zstd \
      -e boot var/cache/apt/archives \
      -wildcards -e 'var/lib/apt/lists/*'
  else
    log "WARN: mksquashfs not available on the host; skipping live squashfs"
  fi

  log "pack rootfs ext4 -> $OUT"
  mkdir -p "$(dirname "$OUT")" "$(dirname "$BOOT_OUT")"
  local rootfs_size_mb=4096
  truncate -s "${rootfs_size_mb}M" "$OUT"
  mkfs.ext4 -q -L OS "$OUT"
  local mnt
  mnt=$(mktemp -d)
  mount -o loop "$OUT" "$mnt"
  cp -a "$work/." "$mnt/"
  # Separate /boot out: move kernel + initrd into boot.img
  local boot_size_mb=512
  truncate -s "${boot_size_mb}M" "$BOOT_OUT"
  mkfs.ext4 -q -L Boot "$BOOT_OUT"
  local boot_mnt
  boot_mnt=$(mktemp -d)
  mount -o loop "$BOOT_OUT" "$boot_mnt"
  if [[ -d "$mnt/boot" ]]; then
    cp -a "$mnt/boot/." "$boot_mnt/"
    # Export the kernel + initrd to well-known paths so the ISO stage
    # can pick them up. linux-image-amd64 places the real files as
    # /boot/vmlinuz-<ver> and /boot/initrd.img-<ver>. The /boot/vmlinuz
    # convenience symlink is maintained by linux-update-symlinks but
    # isn't always present (e.g. linux-base not installed pre-kernel).
    # Glob for the versioned files and pick the newest.
    local out_dir kern_src initrd_src
    out_dir=$(dirname "$OUT")
    kern_src=$(ls -1t "$mnt"/boot/vmlinuz-* 2>/dev/null | head -1)
    initrd_src=$(ls -1t "$mnt"/boot/initrd.img-* 2>/dev/null | head -1)
    if [[ -n "$kern_src" ]]; then
      cp -L "$kern_src" "$out_dir/kernel.vmlinuz"
    fi
    if [[ -n "$initrd_src" ]]; then
      cp -L "$initrd_src" "$out_dir/kernel.initrd"
    fi
    rm -rf "$mnt/boot"/*
  fi
  umount "$boot_mnt"
  umount "$mnt"
  rmdir "$mnt" "$boot_mnt"
  log "layered image at $OUT, boot at $BOOT_OUT"
  if [[ -f "$(dirname "$OUT")/kernel.vmlinuz" ]]; then
    log "exported kernel/initrd to $(dirname "$OUT")/kernel.{vmlinuz,initrd}"
  else
    log "WARN: kernel symlinks not found in layered rootfs — ISO boot will fail"
  fi
}

case "$STAGE" in
  base)    build_base ;;
  layered) build_layered ;;
  *)       echo "unknown stage: $STAGE" >&2; exit 2 ;;
esac
