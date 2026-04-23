#!/usr/bin/env bash
# Package rootfs + boot images into a signed RAUC .raucb bundle.
# CI produces unsigned bundles (cert is the placeholder leaf); real releases
# are re-signed offline via sign-release.sh.

set -euo pipefail

VERSION=""
CHANNEL=""
ROOTFS_IMG=""
BOOT_IMG=""
MANIFEST_TMPL=""
CERT=""
KEY=""
OUT=""

usage() {
  cat <<EOF
Usage: $(basename "$0") --version=X --channel=Y --rootfs=R --boot=B --manifest=M --cert=C --key=K --out=O
EOF
}

for arg in "$@"; do
  case "$arg" in
    --version=*)  VERSION="${arg#*=}" ;;
    --channel=*)  CHANNEL="${arg#*=}" ;;
    --rootfs=*)   ROOTFS_IMG="${arg#*=}" ;;
    --boot=*)     BOOT_IMG="${arg#*=}" ;;
    --manifest=*) MANIFEST_TMPL="${arg#*=}" ;;
    --cert=*)     CERT="${arg#*=}" ;;
    --key=*)      KEY="${arg#*=}" ;;
    --out=*)      OUT="${arg#*=}" ;;
    -h|--help)    usage; exit 0 ;;
    *) echo "unknown arg: $arg" >&2; usage >&2; exit 2 ;;
  esac
done

for v in VERSION CHANNEL ROOTFS_IMG BOOT_IMG MANIFEST_TMPL CERT KEY OUT; do
  [[ -n "${!v}" ]] || { echo "--${v,,} required" >&2; exit 2; }
done

command -v rauc >/dev/null 2>&1 || { echo "rauc not installed" >&2; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { echo "sha256sum missing" >&2; exit 1; }
command -v openssl >/dev/null 2>&1 || { echo "openssl missing" >&2; exit 1; }

log() { printf '[build-bundle] %s\n' "$*"; }

# The repo ships a placeholder cert.pem with comments inside the BEGIN/END
# markers so nobody ever accidentally commits a real signing key. When rauc
# can't parse the placeholder, generate a throwaway development cert into a
# tempdir and use that instead. Real releases re-sign offline via
# hack/ci/rauc-sign-release.sh with the air-gapped NovaNas release key.
DEV_CRT_DIR=""
if ! openssl x509 -in "$CERT" -noout 2>/dev/null; then
  log "placeholder cert detected — generating throwaway dev cert"
  DEV_CRT_DIR=$(mktemp -d)
  openssl req -x509 -newkey rsa:4096 -sha256 -days 1 -nodes \
    -subj "/CN=novanas-ci-throwaway" \
    -keyout "$DEV_CRT_DIR/key.pem" \
    -out    "$DEV_CRT_DIR/cert.pem" 2>/dev/null
  CERT="$DEV_CRT_DIR/cert.pem"
  KEY="$DEV_CRT_DIR/key.pem"
fi

log "staging bundle tree"
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE" "${DEV_CRT_DIR:-}"' EXIT

cp "$ROOTFS_IMG" "$STAGE/rootfs.img"
cp "$BOOT_IMG"   "$STAGE/boot.img"

cat > "$STAGE/hooks.sh" <<'HOOKS'
#!/bin/sh
# RAUC install/post-install hooks for NovaNas.
# Invoked by the rauc service on the appliance during `rauc install`.
set -eu

case "$1" in
  slot-install)
    # Called per-slot after the image is written. No custom action needed —
    # overlayfs and first-boot handle everything.
    ;;
  slot-post-install)
    # Flip the GRUB default to the newly-written slot. RAUC handles this
    # automatically when bootloader=grub, listed here only for clarity.
    ;;
  *) ;;
esac
HOOKS
chmod 0755 "$STAGE/hooks.sh"

ROOTFS_SIZE=$(stat -c%s "$STAGE/rootfs.img")
BOOT_SIZE=$(stat -c%s "$STAGE/boot.img")
ROOTFS_SHA=$(sha256sum "$STAGE/rootfs.img" | awk '{print $1}')
BOOT_SHA=$(sha256sum "$STAGE/boot.img" | awk '{print $1}')
BUILD_ID="${GITHUB_SHA:-$(date -u +%Y%m%d%H%M%S)}"

log "rendering manifest.raucm"
sed \
  -e "s|@VERSION@|$VERSION|g" \
  -e "s|@CHANNEL@|$CHANNEL|g" \
  -e "s|@BUILD_ID@|$BUILD_ID|g" \
  -e "s|@ROOTFS_SIZE@|$ROOTFS_SIZE|g" \
  -e "s|@BOOT_SIZE@|$BOOT_SIZE|g" \
  -e "s|@ROOTFS_SHA256@|$ROOTFS_SHA|g" \
  -e "s|@BOOT_SHA256@|$BOOT_SHA|g" \
  "$MANIFEST_TMPL" > "$STAGE/manifest.raucm"

log "assembling bundle -> $OUT"
mkdir -p "$(dirname "$OUT")"
rm -f "$OUT"

rauc bundle \
  --cert="$CERT" \
  --key="$KEY" \
  "$STAGE" \
  "$OUT"

log "bundle ready: $OUT"
log "NOTE: CI signing is with the placeholder leaf cert. Call sign-release.sh on"
log "      an air-gapped workstation to re-sign with the offline NovaNas release key."
