#!/usr/bin/env bash
# Build a NovaNas installer ISO by repacking debian-trixie-netinst.iso
# with our preseed + late_command + initial RAUC bundle embedded.

set -euo pipefail

# When NETINST_URL is unset, auto-discover the current Debian netinst from
# the published SHA256SUMS (so we don't rot to a 404 every point release).
# Set NETINST_URL+NETINST_SHA256 explicitly for reproducible builds or tests.
NETINST_MIRROR="${NETINST_MIRROR:-https://cdimage.debian.org/cdimage/release/current/amd64/iso-cd}"
NETINST_URL="${NETINST_URL:-}"
NETINST_SHA256="${NETINST_SHA256:-}"
OUT_ISO="${OUT_ISO:-build/out/novanas-installer.iso}"
RAUC_BUNDLE="${RAUC_BUNDLE:-build/out/novanas.raucb}"
WORK_DIR="${WORK_DIR:-build/installer-di-work}"

log() { printf '[build-installer-iso] %s\n' "$*" >&2; }
die() { log "FATAL: $*"; exit 1; }

download_netinst() {
  local cache_dir="netinst-cache"
  mkdir -p "$cache_dir"

  local url sha filename
  if [[ -n "$NETINST_URL" ]]; then
    url="$NETINST_URL"
    filename="$(basename "$url")"
    sha="$NETINST_SHA256"
  else
    log "discovering current netinst from $NETINST_MIRROR/SHA256SUMS"
    curl -fL --retry 3 -o "$cache_dir/SHA256SUMS" "$NETINST_MIRROR/SHA256SUMS" \
      || die "could not fetch SHA256SUMS from $NETINST_MIRROR"
    read -r sha filename < <(grep -E 'debian-[0-9.]+-amd64-netinst\.iso$' "$cache_dir/SHA256SUMS" | head -1)
    [[ -n "$filename" ]] || die "no netinst entry in $cache_dir/SHA256SUMS"
    url="$NETINST_MIRROR/$filename"
    log "discovered $filename ($sha)"
  fi

  local cache="$cache_dir/$filename"
  if [[ ! -f "$cache" ]]; then
    log "fetching $url"
    curl -fL --retry 3 -o "$cache.tmp" "$url" \
      || { rm -f "$cache.tmp"; die "download failed: $url"; }
    mv "$cache.tmp" "$cache"
  fi

  if [[ -n "$sha" ]]; then
    local got
    got=$(sha256sum "$cache" | awk '{print $1}')
    if [[ "$got" != "$sha" ]]; then
      log "checksum mismatch: expected $sha, got $got"
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
  # Bash's set -e doesn't propagate failure of $() into assignment; check
  # explicitly so a 404 / network error stops the build instead of silently
  # passing an empty path to unpack_iso.
  ISO_PATH=$(download_netinst) || die "download_netinst failed"
  [[ -n "$ISO_PATH" && -f "$ISO_PATH" ]] || die "downloaded ISO missing: '$ISO_PATH'"
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
