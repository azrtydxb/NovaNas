#!/usr/bin/env bash
# Seeds sample NovaNas CRs so the UI has something to display.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SAMPLES_DIR="${SCRIPT_DIR}/samples"
KUBECONFIG_RAW="${HOME}/.kube/novanas-dev.kubeconfig.raw"

if [ ! -f "${KUBECONFIG_RAW}" ]; then
  echo "Raw kubeconfig not found at ${KUBECONFIG_RAW}. Run create-cluster.sh first." >&2
  exit 1
fi

export KUBECONFIG="${KUBECONFIG_RAW}"

echo "Applying sample resources from ${SAMPLES_DIR}..."
kubectl apply -f "${SAMPLES_DIR}"

echo "Sample resources applied:"
kubectl get storagepools,datasets,shares,users 2>/dev/null || true
