#!/usr/bin/env bash
# Convert the mkosi-built rootfs directory into the artefacts the rest of the
# NovaNas pipeline consumes:
#
#   - filesystem.squashfs   live-boot rootfs for the installer ISO
#   - rootfs.img            ext4 image for the RAUC bundle
#   - boot.img              ext4 image carrying kernel+initrd for RAUC
#   - kernel.vmlinuz        exported kernel for the ISO's /boot
#   - kernel.initrd         exported initrd for the ISO's /boot
#
# Inputs:
#   --rootfs-dir=<path>     mkosi output directory (rootfs tree)
#   --profile=live|update   live adds squashfs + keeps journald drop-in;
#                           update only emits ext4 images (no squashfs).
#   --out-dir=<path>        where to write artefacts
#
# Replaces the tail of the old os/build/build-rootfs.sh (stage=layered).

set -euo pipefail

ROOTFS_DIR=""
PROFILE="live"
OUT_DIR=""

for arg in "$@"; do
  case "$arg" in
    --rootfs-dir=*) ROOTFS_DIR="${arg#*=}" ;;
    --profile=*)    PROFILE="${arg#*=}" ;;
    --out-dir=*)    OUT_DIR="${arg#*=}" ;;
    -h|--help)
      sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

[[ -n "$ROOTFS_DIR" && -d "$ROOTFS_DIR" ]] || { echo "--rootfs-dir missing" >&2; exit 2; }
[[ -n "$OUT_DIR" ]] || { echo "--out-dir required" >&2; exit 2; }
mkdir -p "$OUT_DIR"

log() { printf '[build-images:%s] %s\n' "$PROFILE" "$*"; }

need_root() {
  if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    echo "requires root (loop mount + chown)" >&2
    exit 1
  fi
}

export_kernel() {
  local kern initrd
  kern=$(ls -1t "$ROOTFS_DIR"/boot/vmlinuz-* 2>/dev/null | head -1 || true)
  initrd=$(ls -1t "$ROOTFS_DIR"/boot/initrd.img-* 2>/dev/null | head -1 || true)
  [[ -n "$kern"   ]] || { echo "no kernel in $ROOTFS_DIR/boot" >&2; exit 1; }
  [[ -n "$initrd" ]] || { echo "no initrd in $ROOTFS_DIR/boot" >&2; exit 1; }
  cp -L "$kern"   "$OUT_DIR/kernel.vmlinuz"
  cp -L "$initrd" "$OUT_DIR/kernel.initrd"
  log "exported kernel.vmlinuz + kernel.initrd"
}

build_squashfs() {
  command -v mksquashfs >/dev/null 2>&1 || { echo "mksquashfs missing" >&2; exit 1; }
  local out="$OUT_DIR/filesystem.squashfs"
  rm -f "$out"
  log "pack squashfs -> $out"
  # Keep proc/sys/dev/run/tmp as empty dirs (live-boot bind-mounts host
  # versions on top). Exclude only /boot (kernel+initrd ship on the ISO) and
  # the apt cache.
  mksquashfs "$ROOTFS_DIR" "$out" \
    -noappend -comp zstd \
    -e boot var/cache/apt/archives \
    -wildcards -e 'var/lib/apt/lists/*'
}

build_ext4_rootfs() {
  need_root
  local out="$OUT_DIR/rootfs.img"
  local size_mb=4096
  log "pack ext4 rootfs -> $out (${size_mb}M)"
  rm -f "$out"
  truncate -s "${size_mb}M" "$out"
  mkfs.ext4 -q -L OS "$out"
  local mnt
  mnt=$(mktemp -d)
  mount -o loop "$out" "$mnt"
  cp -a "$ROOTFS_DIR/." "$mnt/"
  # RAUC slot image does not carry its own kernel/initrd (those live in
  # boot.img); remove to keep rootfs.img compact.
  rm -rf "$mnt/boot"/*
  umount "$mnt"
  rmdir "$mnt"
}

build_ext4_boot() {
  need_root
  local out="$OUT_DIR/boot.img"
  local size_mb=512
  log "pack ext4 boot -> $out (${size_mb}M)"
  rm -f "$out"
  truncate -s "${size_mb}M" "$out"
  mkfs.ext4 -q -L Boot "$out"
  local mnt
  mnt=$(mktemp -d)
  mount -o loop "$out" "$mnt"
  if [[ -d "$ROOTFS_DIR/boot" ]]; then
    cp -a "$ROOTFS_DIR/boot/." "$mnt/"
  fi
  umount "$mnt"
  rmdir "$mnt"
}

case "$PROFILE" in
  live)
    export_kernel
    build_squashfs
    ;;
  update)
    export_kernel
    build_ext4_rootfs
    build_ext4_boot
    ;;
  *)
    echo "unknown profile: $PROFILE (expected live|update)" >&2
    exit 2
    ;;
esac

log "done"
