#!/usr/bin/env bash
#
# teardown.sh — delete the kind cluster created by bootstrap-cluster.sh and
# remove E2E-managed loopback disks / artifacts.
#
set -euo pipefail

CLUSTER="${CLUSTER:-novanas-e2e}"
KEEP_ARTIFACTS="${KEEP_ARTIFACTS:-0}"

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if command -v kind >/dev/null && kind get clusters | grep -qx "${CLUSTER}"; then
  echo "[teardown] deleting kind cluster ${CLUSTER}"
  kind delete cluster --name "${CLUSTER}"
else
  echo "[teardown] no kind cluster ${CLUSTER} to delete"
fi

if [[ "${KEEP_ARTIFACTS}" != "1" ]]; then
  rm -rf "${HERE}/../qemu/artifacts" "${HERE}/../playwright-report" \
         "${HERE}/../test-results" "${HERE}/../compat/s3/.cache" \
         "${HERE}/../compat/nfs/.cache"
  echo "[teardown] cleaned artifacts"
fi

echo "[teardown] done"
