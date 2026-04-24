#!/usr/bin/env bash
# Thin xorriso/grub-mkrescue wrapper that produces a hybrid (UEFI + BIOS)
# Debian Live installer ISO from the mkosi-built live rootfs.
#
# mkosi v25 does not natively produce a Debian Live hybrid ISO (its Format=disk
# builds a UEFI GPT disk image with systemd-boot/grub but no live-boot
# squashfs layer). So we keep this ~70-line wrapper: it consumes the
# build-images.sh outputs (filesystem.squashfs, kernel.vmlinuz, kernel.initrd)
# and emits the hybrid ISO with our grub.cfg.
#
# Inputs:
#   --version, --channel       stamped into the ISO + grub menu entries
#   --installer-binary=<path>  Go installer copied into /novanas/installer
#   --squashfs=<path>          live rootfs
#   --kernel=<path>            vmlinuz
#   --initrd=<path>            initrd.img
#   --out=<path>               ISO output

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OS_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

VERSION=""
CHANNEL=""
INSTALLER_BINARY=""
SQUASHFS=""
KERNEL=""
INITRD=""
OUT=""

for arg in "$@"; do
  case "$arg" in
    --version=*)          VERSION="${arg#*=}" ;;
    --channel=*)          CHANNEL="${arg#*=}" ;;
    --installer-binary=*) INSTALLER_BINARY="${arg#*=}" ;;
    --squashfs=*)         SQUASHFS="${arg#*=}" ;;
    --kernel=*)           KERNEL="${arg#*=}" ;;
    --initrd=*)           INITRD="${arg#*=}" ;;
    --out=*)              OUT="${arg#*=}" ;;
    -h|--help)            sed -n '2,18p' "$0"; exit 0 ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

for v in VERSION CHANNEL INSTALLER_BINARY SQUASHFS KERNEL INITRD OUT; do
  [[ -n "${!v}" ]] || { echo "--${v,,} required" >&2; exit 2; }
done
for f in "$INSTALLER_BINARY" "$SQUASHFS" "$KERNEL" "$INITRD"; do
  [[ -f "$f" ]] || { echo "missing input: $f" >&2; exit 1; }
done
for t in xorriso grub-mkrescue; do
  command -v "$t" >/dev/null 2>&1 || { echo "missing tool: $t" >&2; exit 1; }
done

log() { printf '[build-iso] %s\n' "$*"; }

STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

log "assembling ISO root at $STAGE"
mkdir -p "$STAGE/boot/grub" "$STAGE/novanas" "$STAGE/live"

install -m 0755 "$INSTALLER_BINARY" "$STAGE/novanas/installer"
echo "$VERSION" > "$STAGE/novanas/version"
echo "$CHANNEL" > "$STAGE/novanas/channel"

install -m 0644 "$SQUASHFS" "$STAGE/live/filesystem.squashfs"
install -m 0644 "$KERNEL"   "$STAGE/boot/vmlinuz"
install -m 0644 "$INITRD"   "$STAGE/boot/initrd.img"

# Kernel cmdline: identical to the pre-mkosi build. Journald no-rate-limit
# drop-in is baked into the live rootfs; debug_shell=1 keeps a root shell on
# tty9 for stuck-boot forensics. See os/mkosi.extra-live/etc/systemd/journald.conf.d/.
cat > "$STAGE/boot/grub/grub.cfg" <<EOF
set timeout=5
set default=0

insmod all_video
insmod gfxterm
terminal_output --append gfxterm
terminal_output --append console
terminal_input  --append console

menuentry "Install NovaNas ${VERSION} (${CHANNEL})" {
  linux /boot/vmlinuz boot=live components toram novanas.installer=1 console=tty0 console=ttyS0,115200n8 systemd.getty_auto=0 systemd.unit=multi-user.target systemd.debug_shell=1 systemd.log_level=debug systemd.log_target=kmsg systemd.log_time=1 systemd.show_status=1 random.trust_cpu=on random.trust_bootloader=on
  initrd /boot/initrd.img
}

menuentry "Install NovaNas ${VERSION} (serial console)" {
  linux /boot/vmlinuz boot=live components toram novanas.installer=1 console=ttyS0,115200n8 systemd.getty_auto=0 systemd.unit=multi-user.target random.trust_cpu=on random.trust_bootloader=on
  initrd /boot/initrd.img
}

menuentry "Rescue shell" {
  linux /boot/vmlinuz boot=live components toram single console=tty0 console=ttyS0,115200n8 systemd.getty_auto=0 systemd.unit=multi-user.target random.trust_cpu=on random.trust_bootloader=on
  initrd /boot/initrd.img
}
EOF

log "building hybrid UEFI+BIOS ISO -> $OUT"
mkdir -p "$(dirname "$OUT")"
VOLID="NOVANAS_$(echo "$VERSION" | tr -c '[:alnum:]' '_')"
VOLID="${VOLID:0:32}"

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
