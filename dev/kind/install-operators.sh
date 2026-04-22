#!/usr/bin/env bash
# Builds (if needed), loads, and deploys the NovaNas operators into the
# novanas-dev kind cluster. Uses imagePullPolicy=Never so the locally loaded
# image is always used.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OP_DIR="${REPO_ROOT}/packages/operators"
MANAGER_YAML="${OP_DIR}/config/manager/manager.yaml"
RBAC_YAML="${OP_DIR}/config/rbac/role.yaml"
CLUSTER_NAME="novanas-dev"
IMAGE="novanas/operators:dev"
KUBECONFIG_RAW="${HOME}/.kube/novanas-dev.kubeconfig.raw"

if [ ! -f "${KUBECONFIG_RAW}" ]; then
  echo "Raw kubeconfig not found at ${KUBECONFIG_RAW}. Run create-cluster.sh first." >&2
  exit 1
fi

export KUBECONFIG="${KUBECONFIG_RAW}"

# --- build + load the image -------------------------------------------------
if ! docker image inspect "${IMAGE}" >/dev/null 2>&1; then
  echo "Building ${IMAGE}..."
  docker build -t "${IMAGE}" "${OP_DIR}"
fi

echo "Loading ${IMAGE} into kind cluster '${CLUSTER_NAME}'..."
kind load docker-image "${IMAGE}" --name "${CLUSTER_NAME}"

# --- apply RBAC + manifest with patched image/pullPolicy --------------------
echo "Applying operator RBAC..."
kubectl apply -f "${RBAC_YAML}"

echo "Applying operator Deployment (image=${IMAGE}, pullPolicy=Never)..."
# sed rewrite is robust and dependency-free. We:
#   - replace the image line under containers[0]
#   - force imagePullPolicy: Never
sed -e "s|image: .*operators:.*|image: ${IMAGE}|" \
    -e "s|imagePullPolicy: .*|imagePullPolicy: Never|" \
    "${MANAGER_YAML}" \
  | kubectl apply -f -

echo "Waiting for operator Deployment to become Ready..."
kubectl -n novanas-system rollout status deployment/novanas-operators --timeout=180s

echo "Operators are running."
