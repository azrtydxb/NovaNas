#!/usr/bin/env bash
#
# smbtorture.sh — run the Samba smbtorture suite against the NovaNas SMB
# service. Exercises SMB2/SMB3 protocol-level conformance.
#
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ART="${HERE}/../../artifacts/smb"
mkdir -p "${ART}"

SERVER="${SMB_SERVER:-localhost}"
SHARE="${SMB_SHARE:-e2e-share-photos}"
USER="${SMB_USER:-e2e}"
PASS="${SMB_PASS:-e2e-password}"

command -v smbtorture >/dev/null || {
  echo "smbtorture not installed (apt-get install samba-testsuite)" >&2; exit 2; }

# Protocol + core suites that NovaNas is expected to pass. We deliberately
# skip SMB1 (not enabled), replication-specific tests, and kernel-oplock
# tests that require a Windows server.
SUITES=(
  smb2.create smb2.read smb2.write smb2.lock smb2.getinfo smb2.setinfo
  smb2.durable-open smb2.session
  raw.read raw.write
)

LOG="${ART}/smbtorture.log"
: > "${LOG}"
for s in "${SUITES[@]}"; do
  echo "[smbtorture] running ${s}" | tee -a "${LOG}"
  smbtorture "//${SERVER}/${SHARE}" \
    --user="${USER}%${PASS}" \
    --option="torture:progress=no" \
    "${s}" 2>&1 | tee -a "${LOG}"
done

grep -E '^(FAILED|ERROR)' "${LOG}" && { echo "[smbtorture] FAIL"; exit 1; } || true
echo "[smbtorture] PASS — log in ${LOG}"
