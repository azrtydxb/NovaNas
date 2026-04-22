#!/usr/bin/env bash
#
# smoke-boot.sh — boot a NovaNas ISO in QEMU, drive the installer, and verify
# that k3s + novanas-api come up and the dashboard is reachable.
#
# Inputs (env):
#   ISO               Path to novanas-*.iso. Default: artifacts/novanas.iso
#   DISK_SIZE         Virtual disk size for the target. Default: 40G
#   MEMORY_MB         VM memory. Default: 6144
#   VCPUS             vCPU count. Default: 4
#   SSH_PORT          Host port forwarded to VM :22. Default: 2222
#   UI_PORT           Host port forwarded to VM :443. Default: 8443
#   TIMEOUT_SEC       Overall timeout. Default: 1200 (20 min)
#   QEMU_ACCEL        "kvm" (default on Linux) or "tcg" fallback.
#
# Artifacts (written to qemu/artifacts/):
#   serial.log        Full serial console capture
#   vm-disk.qcow2     Target disk (preserved on failure for inspection)
#
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARTIFACTS="${HERE}/artifacts"
mkdir -p "${ARTIFACTS}"

ISO="${ISO:-${HERE}/../../os/build/novanas.iso}"
DISK_SIZE="${DISK_SIZE:-40G}"
MEMORY_MB="${MEMORY_MB:-6144}"
VCPUS="${VCPUS:-4}"
SSH_PORT="${SSH_PORT:-2222}"
UI_PORT="${UI_PORT:-8443}"
TIMEOUT_SEC="${TIMEOUT_SEC:-1200}"
QEMU_ACCEL="${QEMU_ACCEL:-kvm}"

if [[ ! -f "${ISO}" ]]; then
  echo "ERROR: ISO not found at ${ISO}" >&2
  echo "Build one with 'make -C os iso' or set ISO=/path/to/novanas.iso" >&2
  exit 2
fi

command -v qemu-system-x86_64 >/dev/null || {
  echo "qemu-system-x86_64 not found" >&2
  exit 2
}
command -v qemu-img >/dev/null || { echo "qemu-img not found" >&2; exit 2; }

DISK="${ARTIFACTS}/vm-disk.qcow2"
SERIAL="${ARTIFACTS}/serial.log"

echo "[smoke-boot] creating ${DISK_SIZE} disk at ${DISK}"
qemu-img create -f qcow2 "${DISK}" "${DISK_SIZE}" >/dev/null

ACCEL_FLAGS=()
if [[ "${QEMU_ACCEL}" == "kvm" && -e /dev/kvm ]]; then
  ACCEL_FLAGS=(-enable-kvm -cpu host)
else
  echo "[smoke-boot] falling back to TCG (no KVM)"
  ACCEL_FLAGS=(-cpu qemu64)
fi

echo "[smoke-boot] starting QEMU (timeout ${TIMEOUT_SEC}s)"
# shellcheck disable=SC2086
timeout --foreground "${TIMEOUT_SEC}" qemu-system-x86_64 \
  "${ACCEL_FLAGS[@]}" \
  -m "${MEMORY_MB}" -smp "${VCPUS}" \
  -drive file="${DISK}",if=virtio,format=qcow2 \
  -cdrom "${ISO}" -boot order=d \
  -nic user,model=virtio-net-pci,hostfwd=tcp::"${SSH_PORT}"-:22,hostfwd=tcp::"${UI_PORT}"-:443 \
  -display none \
  -serial "file:${SERIAL}" \
  -monitor none \
  -nographic &
QEMU_PID=$!

trap 'kill "${QEMU_PID}" 2>/dev/null || true' EXIT

# Wait for the UI port to become reachable. The installer will run
# unattended via kernel cmdline preseed (shipped in the ISO).
DEADLINE=$(( $(date +%s) + TIMEOUT_SEC ))
echo "[smoke-boot] waiting for UI on https://localhost:${UI_PORT}"
while (( $(date +%s) < DEADLINE )); do
  if curl -ksSf --max-time 5 "https://localhost:${UI_PORT}/health" >/dev/null; then
    echo "[smoke-boot] UI is up"
    break
  fi
  sleep 10
done

if ! curl -ksSf --max-time 5 "https://localhost:${UI_PORT}/health" >/dev/null; then
  echo "[smoke-boot] TIMEOUT — /health did not respond"
  echo "[smoke-boot] last 200 lines of serial log:"
  tail -n 200 "${SERIAL}" || true
  exit 1
fi

# Additional assertions: version endpoint, pools list (unauthenticated is OK
# to return 401 — we only check that the route exists).
curl -ksSf --max-time 5 "https://localhost:${UI_PORT}/api/version" >/dev/null
STATUS=$(curl -ksS -o /dev/null -w '%{http_code}' "https://localhost:${UI_PORT}/api/v1/pools")
if [[ "${STATUS}" != "200" && "${STATUS}" != "401" ]]; then
  echo "[smoke-boot] unexpected /api/v1/pools status: ${STATUS}"
  exit 1
fi

echo "[smoke-boot] PASS"
