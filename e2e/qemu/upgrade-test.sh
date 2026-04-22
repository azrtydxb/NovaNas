#!/usr/bin/env bash
#
# upgrade-test.sh — installs a prior NovaNas release, applies the current
# release's RAUC bundle, reboots, then verifies state is preserved.
#
# Inputs (env):
#   OLD_ISO        Path to prior-release ISO (e.g. 26.04 LTS). REQUIRED.
#   NEW_BUNDLE     Path to current-release *.raucb. REQUIRED.
#   SSH_PORT       Host port forwarded to VM :22. Default: 2222
#   UI_PORT        Host port forwarded to VM :443. Default: 8443
#   SSH_KEY        Private key used to reach the VM post-install.
#
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARTIFACTS="${HERE}/artifacts/upgrade"
mkdir -p "${ARTIFACTS}"

: "${OLD_ISO:?OLD_ISO must be set to a prior-release ISO}"
: "${NEW_BUNDLE:?NEW_BUNDLE must be set to a *.raucb}"
SSH_PORT="${SSH_PORT:-2222}"
UI_PORT="${UI_PORT:-8443}"
SSH_KEY="${SSH_KEY:-${HOME}/.ssh/id_ed25519}"
SSH_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i "${SSH_KEY}" -p "${SSH_PORT}")

echo "[upgrade] stage 1 — install prior release from ${OLD_ISO}"
ISO="${OLD_ISO}" UI_PORT="${UI_PORT}" SSH_PORT="${SSH_PORT}" \
  bash "${HERE}/smoke-boot.sh"

echo "[upgrade] stage 2 — seed marker state"
# Write a marker file so we can verify persistence across upgrade+reboot.
ssh "${SSH_OPTS[@]}" novanas@localhost \
  'sudo mkdir -p /var/lib/novanas/e2e && echo upgrade-test-marker | sudo tee /var/lib/novanas/e2e/marker >/dev/null'

# Record the pre-upgrade version for comparison.
PRE_VERSION=$(curl -ksS "https://localhost:${UI_PORT}/api/version" | grep -o '"version":"[^"]*"' || true)
echo "[upgrade] pre-upgrade version: ${PRE_VERSION}"

echo "[upgrade] stage 3 — upload and apply RAUC bundle ${NEW_BUNDLE}"
scp "${SSH_OPTS[@]/-p/-P}" "${NEW_BUNDLE}" novanas@localhost:/tmp/update.raucb
ssh "${SSH_OPTS[@]}" novanas@localhost 'sudo rauc install /tmp/update.raucb'

echo "[upgrade] stage 4 — reboot into new slot"
ssh "${SSH_OPTS[@]}" novanas@localhost 'sudo systemctl reboot || true' || true

echo "[upgrade] waiting for UI to return after reboot"
DEADLINE=$(( $(date +%s) + 900 ))
while (( $(date +%s) < DEADLINE )); do
  if curl -ksSf --max-time 5 "https://localhost:${UI_PORT}/health" >/dev/null; then
    break
  fi
  sleep 10
done
curl -ksSf --max-time 5 "https://localhost:${UI_PORT}/health" >/dev/null || {
  echo "[upgrade] FAIL — UI did not return after upgrade reboot"; exit 1; }

echo "[upgrade] stage 5 — verify marker preserved"
MARKER=$(ssh "${SSH_OPTS[@]}" novanas@localhost 'cat /var/lib/novanas/e2e/marker' || echo missing)
if [[ "${MARKER}" != "upgrade-test-marker" ]]; then
  echo "[upgrade] FAIL — marker not preserved: got ${MARKER}"
  exit 1
fi

POST_VERSION=$(curl -ksS "https://localhost:${UI_PORT}/api/version" | grep -o '"version":"[^"]*"' || true)
echo "[upgrade] post-upgrade version: ${POST_VERSION}"

if [[ "${PRE_VERSION}" == "${POST_VERSION}" && -n "${PRE_VERSION}" ]]; then
  echo "[upgrade] FAIL — version did not change after RAUC install"
  exit 1
fi

echo "[upgrade] PASS"
