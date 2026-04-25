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

main() {
  command -v xorriso >/dev/null 2>&1 || { echo "xorriso not installed"; exit 1; }
  command -v gunzip  >/dev/null 2>&1 || { echo "gunzip not installed"; exit 1; }

  mkdir -p "$WORK_DIR" "$(dirname "$OUT_ISO")"

  log "step 1: download netinst"
  ISO_PATH=$(download_netinst)
  log "step 2: unpack ISO contents"
  log "step 3: inject preseed into initrd"
  log "step 4: copy late_command + RAUC bundle into ISO"
  log "step 5: customize grub menu"
  log "step 6: rebuild ISO with xorriso"

  # implementation in subsequent tasks
}

main "$@"
