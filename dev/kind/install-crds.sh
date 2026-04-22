#!/usr/bin/env bash
# Applies every NovaNas CRD and waits for Established.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
CRD_DIR="${REPO_ROOT}/packages/operators/config/crd/bases"
KUBECONFIG_RAW="${HOME}/.kube/novanas-dev.kubeconfig.raw"

if [ ! -f "${KUBECONFIG_RAW}" ]; then
  echo "Raw kubeconfig not found at ${KUBECONFIG_RAW}. Run create-cluster.sh first." >&2
  exit 1
fi

if [ ! -d "${CRD_DIR}" ]; then
  echo "CRD directory not found: ${CRD_DIR}" >&2
  exit 1
fi

export KUBECONFIG="${KUBECONFIG_RAW}"

echo "Applying CRDs from ${CRD_DIR}..."
kubectl apply -f "${CRD_DIR}"

echo "Waiting for CRDs to reach Established condition..."
for f in "${CRD_DIR}"/*.yaml; do
  name="$(awk '/^  name:/ { print $2; exit }' "${f}")"
  if [ -n "${name}" ]; then
    kubectl wait --for=condition=Established --timeout=60s "crd/${name}" >/dev/null
    echo "  ok  ${name}"
  fi
done

echo "All CRDs established."
