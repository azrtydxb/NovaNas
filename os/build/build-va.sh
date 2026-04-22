#!/usr/bin/env bash
# Build virtual appliance images. Drives Packer + qemu-img to produce:
#   novanas-<ver>.raw       (used as pivot to all other formats)
#   novanas-<ver>.qcow2     (KVM / Proxmox)
#   novanas-<ver>.vmdk      (VMware Workstation / Fusion)
#   novanas-<ver>.ova       (VMware vSphere / ESXi)
#
# Packer drives a headless KVM VM that boots the installer ISO, runs an
# automated install to a virtual disk, shuts down cleanly. We then convert.

set -euo pipefail

VERSION=""
CHANNEL=""
ISO=""
PACKER_DIR=""
OUT_DIR=""

usage() {
  cat <<EOF
Usage: $(basename "$0") --version=X --channel=Y --iso=ISO --packer-dir=DIR --out-dir=DIR
EOF
}

for arg in "$@"; do
  case "$arg" in
    --version=*)    VERSION="${arg#*=}" ;;
    --channel=*)    CHANNEL="${arg#*=}" ;;
    --iso=*)        ISO="${arg#*=}" ;;
    --packer-dir=*) PACKER_DIR="${arg#*=}" ;;
    --out-dir=*)    OUT_DIR="${arg#*=}" ;;
    -h|--help)      usage; exit 0 ;;
    *) echo "unknown arg: $arg" >&2; usage >&2; exit 2 ;;
  esac
done

for v in VERSION CHANNEL ISO PACKER_DIR OUT_DIR; do
  [[ -n "${!v}" ]] || { echo "--${v,,} required" >&2; exit 2; }
done

[[ -f "$ISO" ]] || { echo "missing ISO: $ISO" >&2; exit 1; }
command -v packer   >/dev/null 2>&1 || { echo "packer not installed"   >&2; exit 1; }
command -v qemu-img >/dev/null 2>&1 || { echo "qemu-img not installed" >&2; exit 1; }

log() { printf '[build-va] %s\n' "$*"; }

mkdir -p "$OUT_DIR"

RAW="$OUT_DIR/novanas-$VERSION.raw"
QCOW2="$OUT_DIR/novanas-$VERSION.qcow2"
VMDK="$OUT_DIR/novanas-$VERSION.vmdk"
OVA="$OUT_DIR/novanas-$VERSION.ova"

log "packer init + validate"
packer init "$PACKER_DIR/"
packer validate \
  -var "version=$VERSION" \
  -var "channel=$CHANNEL" \
  -var "iso_path=$ISO" \
  -var "out_dir=$OUT_DIR" \
  "$PACKER_DIR/"

log "packer build (headless KVM -> raw disk)"
packer build \
  -var "version=$VERSION" \
  -var "channel=$CHANNEL" \
  -var "iso_path=$ISO" \
  -var "out_dir=$OUT_DIR" \
  "$PACKER_DIR/"

# Packer emits $OUT_DIR/novanas-$VERSION.raw as primary artifact.
[[ -f "$RAW" ]] || { echo "packer did not produce $RAW" >&2; exit 1; }

log "convert raw -> qcow2"
qemu-img convert -O qcow2 -c "$RAW" "$QCOW2"

log "convert raw -> vmdk"
qemu-img convert -O vmdk -o subformat=streamOptimized "$RAW" "$VMDK"

log "package ova (tar of OVF + VMDK)"
OVA_STAGE=$(mktemp -d)
trap 'rm -rf "$OVA_STAGE"' EXIT
cp "$VMDK" "$OVA_STAGE/novanas-$VERSION.vmdk"
cat > "$OVA_STAGE/novanas-$VERSION.ovf" <<OVF
<?xml version="1.0" encoding="UTF-8"?>
<!-- Minimal OVF; production builds use ovftool for full metadata. -->
<Envelope xmlns="http://schemas.dmtf.org/ovf/envelope/1">
  <References>
    <File ovf:id="disk1" ovf:href="novanas-$VERSION.vmdk"/>
  </References>
  <VirtualSystem ovf:id="novanas-$VERSION">
    <Name>NovaNas $VERSION ($CHANNEL)</Name>
  </VirtualSystem>
</Envelope>
OVF
( cd "$OVA_STAGE" && tar -cf "$OVA" "novanas-$VERSION.ovf" "novanas-$VERSION.vmdk" )

log "virtual appliance images ready in $OUT_DIR"
ls -lh "$RAW" "$QCOW2" "$VMDK" "$OVA"
