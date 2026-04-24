#!/usr/bin/env bash
# Build a hybrid (UEFI + BIOS) NovaNas installer ISO.
# The ISO boots GRUB, runs the text-mode installer from A5-Installer, and
# carries the RAUC bundle alongside for fully offline installation.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OS_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

VERSION=""
CHANNEL=""
INSTALLER_BINARY=""
BUNDLE=""
OUT=""

usage() {
  cat <<EOF
Usage: $(basename "$0") --version=X --channel=Y --installer-binary=P --bundle=B --out=O
EOF
}

for arg in "$@"; do
  case "$arg" in
    --version=*)           VERSION="${arg#*=}" ;;
    --channel=*)           CHANNEL="${arg#*=}" ;;
    --installer-binary=*)  INSTALLER_BINARY="${arg#*=}" ;;
    --bundle=*)            BUNDLE="${arg#*=}" ;;
    --out=*)               OUT="${arg#*=}" ;;
    -h|--help)             usage; exit 0 ;;
    *) echo "unknown arg: $arg" >&2; usage >&2; exit 2 ;;
  esac
done

for v in VERSION CHANNEL INSTALLER_BINARY BUNDLE OUT; do
  [[ -n "${!v}" ]] || { echo "--${v,,} required" >&2; exit 2; }
done

for f in "$INSTALLER_BINARY" "$BUNDLE"; do
  [[ -f "$f" ]] || { echo "missing input: $f" >&2; exit 1; }
done

for t in xorriso grub-mkrescue mksquashfs; do
  command -v "$t" >/dev/null 2>&1 || { echo "missing tool: $t" >&2; exit 1; }
done

log() { printf '[build-iso] %s\n' "$*"; }

STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

log "assembling ISO root at $STAGE"
mkdir -p "$STAGE/boot/grub" "$STAGE/novanas" "$STAGE/EFI/BOOT" "$STAGE/live"

install -m 0755 "$INSTALLER_BINARY" "$STAGE/novanas/installer"
# The installer no longer needs the .raucb inside the ISO: it clones
# the live-booted squashfs onto the target partition via unsquashfs.
# The .raucb remains an artifact of the RAUC bundle job for A/B
# update use post-install.
echo "$VERSION" > "$STAGE/novanas/version"
echo "$CHANNEL" > "$STAGE/novanas/channel"

# live-boot expects the squashfs rootfs at /live/filesystem.squashfs by
# default. build-rootfs.sh --stage=layered exports it to build/out/, and
# CI downloads it as part of the rauc-bundle artifact. If it's absent we
# still produce an ISO so earlier pipeline stages can see their failure.
SQUASHFS="$OS_DIR/build/out/filesystem.squashfs"
if [[ -f "$SQUASHFS" ]]; then
  log "placing live rootfs squashfs ($(stat -c%s "$SQUASHFS") bytes)"
  install -m 0644 "$SQUASHFS" "$STAGE/live/filesystem.squashfs"
else
  echo "ERROR: filesystem.squashfs missing at $SQUASHFS — live boot will fail" >&2
  exit 1
fi

cat > "$STAGE/boot/grub/grub.cfg" <<EOF
set timeout=5
set default=0

insmod all_video
insmod gfxterm
terminal_output --append gfxterm
terminal_output --append console
terminal_input  --append console

menuentry "Install NovaNas ${VERSION} (${CHANNEL})" {
  # No 'quiet splash' — text-mode boot. Plymouth captures the console
  # when splash is on and only lets a handful of messages through,
  # which on i915-ENOENT-firmware boards looks like a stalled boot
  # even when it's progressing. Keep the full systemd output visible.
  # systemd.debug-shell=1 opens a root shell on tty9 (Ctrl+Alt+F9) so
  # a stuck boot can still be inspected live.
  linux /boot/vmlinuz boot=live components toram novanas.installer=1 console=tty0 console=ttyS0,115200n8 systemd.getty_auto=0 systemd.unit=multi-user.target systemd.debug_shell=1 random.trust_cpu=on random.trust_bootloader=on
  initrd /boot/initrd.img
}

menuentry "Install NovaNas ${VERSION} (serial console)" {
  # Remote install via IPMI SOL / headless boards with a real UART.
  linux /boot/vmlinuz boot=live components toram novanas.installer=1 console=ttyS0,115200n8 systemd.getty_auto=0 systemd.unit=multi-user.target random.trust_cpu=on random.trust_bootloader=on
  initrd /boot/initrd.img
}

menuentry "Rescue shell" {
  linux /boot/vmlinuz boot=live components toram single console=tty0 console=ttyS0,115200n8 systemd.getty_auto=0 systemd.unit=multi-user.target random.trust_cpu=on random.trust_bootloader=on
  initrd /boot/initrd.img
}

# Diagnostic: full systemd verbose logging to the kernel console. Use
# when boot stalls and we need to see which unit systemd is stuck on.
# systemd.log_level=debug prints every transaction/unit decision.
# systemd.log_target=kmsg routes to /dev/kmsg so the console shows it.
# systemd.log_time=1 timestamps each line.
# systemd.show_status=1 prevents plymouth from hiding unit messages.
menuentry "Debug: verbose systemd boot" {
  linux /boot/vmlinuz boot=live components toram novanas.installer=1 console=tty0 console=ttyS0,115200n8 systemd.getty_auto=0 systemd.unit=multi-user.target random.trust_cpu=on random.trust_bootloader=on systemd.log_level=info systemd.log_target=kmsg systemd.log_time=1 systemd.show_status=1
  initrd /boot/initrd.img
}
EOF

log "placing kernel/initrd stubs (CI injects real ones from the layered image)"
touch "$STAGE/boot/vmlinuz" "$STAGE/boot/initrd.img"
if [[ -f "$OS_DIR/build/out/kernel.vmlinuz" ]]; then
  cp "$OS_DIR/build/out/kernel.vmlinuz" "$STAGE/boot/vmlinuz"
fi
if [[ -f "$OS_DIR/build/out/kernel.initrd" ]]; then
  cp "$OS_DIR/build/out/kernel.initrd"  "$STAGE/boot/initrd.img"
fi

log "building hybrid UEFI+BIOS ISO -> $OUT"
mkdir -p "$(dirname "$OUT")"

# xorriso's ISO 9660 volid limit is 32 chars, alnum + underscore only.
# Replace non-alnum with _ and trim.
VOLID="NOVANAS_$(echo "$VERSION" | tr -c '[:alnum:]' '_')"
VOLID="${VOLID:0:32}"

# grub-mkrescue forwards extra args to xorriso's mkisofs emulation,
# where volume id is '-V' (single dash, space-separated), not --volid.
grub-mkrescue \
  --output="$OUT" \
  "$STAGE" \
  -- \
  -volid "$VOLID" \
  -isohybrid-mbr /usr/lib/ISOLINUX/isohdpfx.bin 2>/dev/null || \
grub-mkrescue \
  --output="$OUT" \
  "$STAGE" \
  -- \
  -volid "$VOLID"

log "ISO ready at $OUT ($(stat -c%s "$OUT") bytes)"
