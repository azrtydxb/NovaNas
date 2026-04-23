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
  trap 'umount -R "$work"/dev "$work"/proc "$work"/sys 2>/dev/null || true; rm -rf "$work"' EXIT

  log "extract base -> $work"
  tar -xpf "$IN" -C "$work"

  log "overlay NovaNas rootfs skeleton"
  cp -a "$OS_DIR/rootfs/." "$work/"

  log "prep chroot bind mounts"
  mount --bind /dev  "$work/dev"
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

  chroot "$work" /bin/bash -eu -o pipefail -c "
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install -y --no-install-recommends $LAYERED_PACKAGES
    apt-get clean
    rm -rf /var/lib/apt/lists/*
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
    locale-gen $LOCALE
    update-locale LANG=$LOCALE
    ln -sf /usr/share/zoneinfo/$TIMEZONE /etc/localtime
    printf '%s\n' '$VERSION' > /etc/novanas/version
    printf '%s\n' '$CHANNEL' > /etc/novanas/channel
  "

  log "unmount chroot binds"
  umount -R "$work"/dev "$work"/proc "$work"/sys

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
    rm -rf "$mnt/boot"/*
  fi
  umount "$boot_mnt"
  umount "$mnt"
  rmdir "$mnt" "$boot_mnt"
  log "layered image at $OUT, boot at $BOOT_OUT"
}

case "$STAGE" in
  base)    build_base ;;
  layered) build_layered ;;
  *)       echo "unknown stage: $STAGE" >&2; exit 2 ;;
esac
