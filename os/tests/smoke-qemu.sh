#!/usr/bin/env bash
# Boot a built NovaNas disk image in QEMU, wait for the system to reach
# multi-user.target, and assert k3s + novanas-api come up. Exits 0 on pass.

set -euo pipefail

IMAGE="${1:-}"
TIMEOUT="${2:-900}"

[[ -n "$IMAGE" && -f "$IMAGE" ]] || {
  echo "Usage: $(basename "$0") <disk-image> [timeout-seconds]" >&2
  exit 2
}

command -v qemu-system-x86_64 >/dev/null 2>&1 || {
  echo "qemu-system-x86_64 not installed" >&2; exit 1; }

log() { printf '[smoke] %s\n' "$*"; }

WORK=$(mktemp -d)
trap 'kill %1 2>/dev/null || true; rm -rf "$WORK"' EXIT

SERIAL="$WORK/serial.log"

# Use a scratch copy so we don't modify the built image.
SCRATCH="$WORK/scratch.qcow2"
qemu-img create -f qcow2 -b "$IMAGE" -F raw "$SCRATCH" >/dev/null

log "booting $IMAGE (serial log: $SERIAL)"
qemu-system-x86_64 \
  -M q35 -enable-kvm -cpu host -smp 2 -m 4096 \
  -drive file="$SCRATCH",if=virtio,format=qcow2 \
  -netdev user,id=n0,hostfwd=tcp::18443-:6443 \
  -device virtio-net,netdev=n0 \
  -nographic \
  -serial file:"$SERIAL" \
  -bios /usr/share/ovmf/OVMF.fd &
QEMU_PID=$!

deadline=$(( $(date +%s) + TIMEOUT ))

wait_for_pattern() {
  local pattern="$1"
  local label="$2"
  while (( $(date +%s) < deadline )); do
    if grep -qE "$pattern" "$SERIAL" 2>/dev/null; then
      log "saw '$label'"
      return 0
    fi
    if ! kill -0 "$QEMU_PID" 2>/dev/null; then
      log "qemu exited before '$label' appeared"
      return 1
    fi
    sleep 3
  done
  log "timed out waiting for '$label'"
  return 1
}

wait_for_pattern 'Reached target (multi-user|Multi-User System)' "multi-user target" || {
  log "FAIL: boot never completed"; exit 1; }

wait_for_pattern 'k3s.*Started|Started k3s' "k3s start" || {
  log "FAIL: k3s never started"; exit 1; }

log "waiting for k3s API on 127.0.0.1:18443"
api_deadline=$(( $(date +%s) + 300 ))
until curl -k --max-time 3 "https://127.0.0.1:18443/readyz" 2>/dev/null | grep -q '^ok'; do
  (( $(date +%s) < api_deadline )) || { log "FAIL: k3s API not ready"; exit 1; }
  sleep 5
done

log "PASS: NovaNas booted, k3s API responsive"
kill "$QEMU_PID" 2>/dev/null || true
exit 0
