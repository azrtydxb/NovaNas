#!/usr/bin/env bash
#
# bootstrap-cluster.sh — spin up a kind cluster and install NovaNas via the
# umbrella Helm chart using the E2E values overlay. Idempotent; re-running
# reuses an existing cluster.
#
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${HERE}/../.." && pwd)"

CLUSTER="${CLUSTER:-novanas-e2e}"
NS="${NS:-novanas-system}"
CHART="${CHART:-${REPO_ROOT}/helm}"
VALUES="${VALUES:-${HERE}/../fixtures/test-values.yaml}"
KIND_CONFIG="${KIND_CONFIG:-${HERE}/../fixtures/kind-cluster.yaml}"
IMAGE_PULL_SECRET="${IMAGE_PULL_SECRET:-}"

for bin in kind kubectl helm; do
  command -v "${bin}" >/dev/null || { echo "${bin} not installed" >&2; exit 2; }
done

if ! kind get clusters | grep -qx "${CLUSTER}"; then
  echo "[bootstrap] creating kind cluster ${CLUSTER}"
  kind create cluster --name "${CLUSTER}" --config "${KIND_CONFIG}" --wait 5m
else
  echo "[bootstrap] reusing existing kind cluster ${CLUSTER}"
fi

kubectl create namespace "${NS}" --dry-run=client -o yaml | kubectl apply -f -

if [[ -n "${IMAGE_PULL_SECRET}" && -f "${IMAGE_PULL_SECRET}" ]]; then
  kubectl -n "${NS}" create secret generic ghcr-pull \
    --from-file=.dockerconfigjson="${IMAGE_PULL_SECRET}" \
    --type=kubernetes.io/dockerconfigjson \
    --dry-run=client -o yaml | kubectl apply -f -
fi

echo "[bootstrap] helm upgrade --install"
helm dependency update "${CHART}" || true
helm upgrade --install novanas "${CHART}" \
  --namespace "${NS}" \
  --values "${VALUES}" \
  --wait --timeout 15m

echo "[bootstrap] waiting for API + UI rollout"
kubectl -n "${NS}" rollout status deploy/novanas-api --timeout=10m
kubectl -n "${NS}" rollout status deploy/novanas-ui  --timeout=10m

echo "[bootstrap] PASS — UI should be reachable via kind extraPortMappings on :8443"
