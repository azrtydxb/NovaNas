#!/usr/bin/env bash
# Deletes the novanas-dev kind cluster and removes its kubeconfig artifacts.

set -euo pipefail

CLUSTER_NAME="novanas-dev"
KUBE_DIR="${HOME}/.kube"

if command -v kind >/dev/null 2>&1 && kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"; then
  echo "Deleting kind cluster '${CLUSTER_NAME}'..."
  kind delete cluster --name "${CLUSTER_NAME}"
else
  echo "kind cluster '${CLUSTER_NAME}' not found — nothing to delete."
fi

rm -f "${KUBE_DIR}/novanas-dev.kubeconfig" "${KUBE_DIR}/novanas-dev.kubeconfig.raw"

# Best-effort cleanup of merged ~/.kube/config entries.
if command -v kubectl >/dev/null 2>&1 && [ -f "${KUBE_DIR}/config" ]; then
  kubectl --kubeconfig="${KUBE_DIR}/config" config delete-context "kind-${CLUSTER_NAME}" >/dev/null 2>&1 || true
  kubectl --kubeconfig="${KUBE_DIR}/config" config delete-cluster "kind-${CLUSTER_NAME}" >/dev/null 2>&1 || true
  kubectl --kubeconfig="${KUBE_DIR}/config" config delete-user "kind-${CLUSTER_NAME}" >/dev/null 2>&1 || true
fi

echo "Uninstall complete."
