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

unpack_iso() {
  local iso="$1"
  local dest="$WORK_DIR/iso"
  rm -rf "$dest"
  mkdir -p "$dest"
  log "extracting $iso to $dest"
  xorriso -osirrox on -indev "$iso" -extract / "$dest"
  chmod -R u+w "$dest"
}

inject_preseed() {
  local initrd="$WORK_DIR/iso/install.amd/initrd.gz"
  local stage="$WORK_DIR/initrd-stage"
  rm -rf "$stage"
  mkdir -p "$stage"
  log "unpacking initrd"
  ( cd "$stage" && gunzip < "../iso/install.amd/initrd.gz" | cpio -id --quiet )
  log "copying preseed into initrd"
  cp installer-di/preseed.cfg "$stage/preseed.cfg"
  log "repacking initrd"
  ( cd "$stage" && find . | cpio --create --format=newc --quiet | gzip > "../iso/install.amd/initrd.gz" )
}

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

copy_payload() {
  local payload_dir="$WORK_DIR/iso/novanas"
  mkdir -p "$payload_dir"
  cp installer-di/late_command.sh "$payload_dir/"
  if [[ -f "$RAUC_BUNDLE" ]]; then
    log "copying RAUC bundle ($(stat -c %s "$RAUC_BUNDLE" 2>/dev/null || stat -f %z "$RAUC_BUNDLE") bytes)"
    cp "$RAUC_BUNDLE" "$payload_dir/novanas-initial.raucb"
  else
    log "WARN: no RAUC bundle at $RAUC_BUNDLE; ISO will install stock Debian"
  fi
  if [[ -f os/rauc/keyring.pem ]]; then
    cp os/rauc/keyring.pem "$payload_dir/novanas-keyring.pem"
  fi
}

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

main() {
  command -v xorriso >/dev/null 2>&1 || { echo "xorriso not installed"; exit 1; }
  command -v gunzip  >/dev/null 2>&1 || { echo "gunzip not installed"; exit 1; }

  mkdir -p "$WORK_DIR" "$(dirname "$OUT_ISO")"

  log "step 1: download netinst"
  ISO_PATH=$(download_netinst)
  log "step 2: unpack ISO contents"
  unpack_iso "$ISO_PATH"
  log "step 3: inject preseed into initrd"
  inject_preseed
  log "step 4: copy late_command + RAUC bundle into ISO"
  copy_payload
  log "step 5: customize GRUB menu"
  customize_grub
  log "step 6: rebuild ISO with xorriso"
  rebuild_iso
  log "DONE: $OUT_ISO ($(stat -c %s "$OUT_ISO" 2>/dev/null || stat -f %z "$OUT_ISO") bytes)"
}

main "$@"
