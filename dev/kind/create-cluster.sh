#!/usr/bin/env bash
# Creates the novanas-dev kind cluster and writes a kubeconfig that is
# reachable from inside Docker containers on the same host (compose api).
#
# Output:
#   $HOME/.kube/novanas-dev.kubeconfig       — rewritten for container use
#   $HOME/.kube/novanas-dev.kubeconfig.raw   — original, host-reachable copy

set -euo pipefail

CLUSTER_NAME="novanas-dev"
CONFIG_FILE="$(cd "$(dirname "$0")" && pwd)/kind-cluster.yaml"
KUBE_DIR="${HOME}/.kube"
RAW_KUBECONFIG="${KUBE_DIR}/novanas-dev.kubeconfig.raw"
KUBECONFIG_OUT="${KUBE_DIR}/novanas-dev.kubeconfig"

command -v kind >/dev/null 2>&1 || { echo "kind is not installed. See https://kind.sigs.k8s.io/" >&2; exit 1; }
command -v kubectl >/dev/null 2>&1 || { echo "kubectl is not installed." >&2; exit 1; }

mkdir -p "${KUBE_DIR}"

if kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"; then
  echo "kind cluster '${CLUSTER_NAME}' already exists — skipping create."
else
  echo "Creating kind cluster '${CLUSTER_NAME}'..."
  kind create cluster --name "${CLUSTER_NAME}" --config "${CONFIG_FILE}" --wait 120s
fi

echo "Exporting kubeconfig to ${RAW_KUBECONFIG}"
kind get kubeconfig --name "${CLUSTER_NAME}" > "${RAW_KUBECONFIG}"

# Rewrite server URL so the compose `api` container can reach the
# kube-apiserver. Inside a container 127.0.0.1 is the container itself;
# Docker Desktop exposes the host as host.docker.internal.
# TLS is skipped because the apiserver cert is issued for 127.0.0.1, not
# host.docker.internal. This is dev-only; never do this in production.
echo "Rewriting kubeconfig for container networking -> ${KUBECONFIG_OUT}"
sed -e 's|https://127\.0\.0\.1:|https://host.docker.internal:|g' \
    -e 's|https://0\.0\.0\.0:|https://host.docker.internal:|g' \
    "${RAW_KUBECONFIG}" \
  | awk '
      /^    certificate-authority-data:/ { next }
      /^    server: https:\/\/host\.docker\.internal:/ {
        print
        print "    insecure-skip-tls-verify: true"
        next
      }
      { print }
    ' > "${KUBECONFIG_OUT}"

# Make sure local kubectl picks up the cluster on the host too — export a
# merged config for interactive use.
KUBECONFIG="${HOME}/.kube/config:${RAW_KUBECONFIG}" kubectl config view --flatten >"${HOME}/.kube/config.merged" 2>/dev/null || true
if [ -s "${HOME}/.kube/config.merged" ]; then
  mv "${HOME}/.kube/config.merged" "${HOME}/.kube/config"
fi
kubectl --kubeconfig="${HOME}/.kube/config" config use-context "kind-${CLUSTER_NAME}" >/dev/null

echo "kind cluster ready."
echo "  raw kubeconfig (host):      ${RAW_KUBECONFIG}"
echo "  container kubeconfig:       ${KUBECONFIG_OUT}"
echo "  current context set to:     kind-${CLUSTER_NAME}"
