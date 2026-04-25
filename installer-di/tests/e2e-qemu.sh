#!/usr/bin/env bash
# Boot the installer ISO in QEMU with two virtual disks; verify install
# completes without prompts and the resulting system boots successfully.
set -euo pipefail

ISO="${1:-build/out/novanas-installer.iso}"
DISK1=$(mktemp /tmp/novanas-disk1.XXXXXX.img)
DISK2=$(mktemp /tmp/novanas-disk2.XXXXXX.img)
trap 'rm -f "$DISK1" "$DISK2"' EXIT
qemu-img create -f raw "$DISK1" 20G
qemu-img create -f raw "$DISK2" 20G

# Phase 1: install
qemu-system-x86_64 \
  -m 2G -enable-kvm \
  -drive "file=$ISO,media=cdrom" \
  -drive "file=$DISK1,if=virtio,format=raw" \
  -drive "file=$DISK2,if=virtio,format=raw" \
  -boot d \
  -nographic \
  -serial mon:stdio \
  -no-reboot \
  -append "auto=true priority=critical preseed/file=/preseed.cfg console=ttyS0,115200n8" \
  | tee install.log

grep -q "late_command done" install.log || {
  echo "FAIL: install did not complete cleanly"
  exit 1
}

# Phase 2: boot the installed system
qemu-system-x86_64 \
  -m 2G -enable-kvm \
  -drive "file=$DISK1,if=virtio,format=raw" \
  -drive "file=$DISK2,if=virtio,format=raw" \
  -nographic -serial mon:stdio \
  -no-reboot \
  | tee boot.log &
QEMU_PID=$!
sleep 60
kill "$QEMU_PID" 2>/dev/null || true

grep -q "novanas login:" boot.log || {
  echo "FAIL: installed system did not reach login prompt"
  exit 1
}

echo "PASS: install + boot succeeded"
