#!/usr/bin/env bash
# Install the NovaNAS CSI driver into the current kubeconfig context.
# Idempotent: safe to re-run after edits to the manifests.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFESTS="${SCRIPT_DIR}/manifests"

err()  { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
info() { printf '==> %s\n' "$*"; }
warn() { printf 'WARNING: %s\n' "$*" >&2; }

command -v kubectl >/dev/null 2>&1 || err "kubectl not found in PATH"

info "Verifying cluster connectivity"
kubectl version --client --output=yaml >/dev/null
kubectl cluster-info >/dev/null || err "cannot reach cluster (check KUBECONFIG)"

# 70-secret-template.yaml is intentionally NOT applied. Warn loudly if the
# operator hasn't created the real Secret yet, or if the Secret is missing
# the new OIDC keys (this catches stale Secrets from the legacy static-token
# era).
if ! kubectl -n nova-csi get secret nova-csi-auth >/dev/null 2>&1; then
    warn "Secret nova-csi-auth/nova-csi does not exist yet."
    warn "The controller and node pods will CrashLoopBackOff until you create it."
    warn "Generate the Keycloak client and create the Secret:"
    warn "  ./deploy/keycloak/create-csi-client.sh > /tmp/csi.json"
    warn "  SECRET=\$(jq -r .clientSecret /tmp/csi.json)"
    warn "  kubectl -n nova-csi create secret generic nova-csi-auth \\"
    warn "    --from-literal=oidc-client-id=nova-csi \\"
    warn "    --from-literal=oidc-client-secret=\"\$SECRET\" \\"
    warn "    --from-file=ca.crt=/etc/nova-ca/ca.crt"
else
    for k in oidc-client-id oidc-client-secret ca.crt; do
        if ! kubectl -n nova-csi get secret nova-csi-auth -o "jsonpath={.data.${k}}" 2>/dev/null | grep -q '.'; then
            warn "Secret nova-csi-auth is missing key '${k}' — pods will fail to start."
            warn "See deploy/csi/manifests/70-secret-template.yaml for the expected shape."
        fi
    done
fi

# Apply in dependency order. Globbing 40-* covers both StorageClass files.
ORDER=(
    "00-namespace.yaml"
    "10-rbac.yaml"
    "20-csidriver.yaml"
    "30-volumesnapshotclass.yaml"
    "40-storageclass-fs.yaml"
    "40-storageclass-block.yaml"
    "50-controller.yaml"
    "60-node.yaml"
)

for f in "${ORDER[@]}"; do
    path="${MANIFESTS}/${f}"
    [[ -f "${path}" ]] || err "missing manifest: ${path}"
    info "Applying ${f}"
    kubectl apply -f "${path}"
done

info "Waiting for controller Deployment to become Ready (timeout 5m)"
kubectl -n nova-csi rollout status deployment/nova-csi-controller --timeout=5m

info "Waiting for node DaemonSet to become Ready (timeout 5m)"
kubectl -n nova-csi rollout status daemonset/nova-csi-node --timeout=5m

info "Pods in nova-csi:"
kubectl -n nova-csi get pods -o wide

info "CSIDriver:"
kubectl get csidriver csi.novanas.io

info "StorageClasses:"
kubectl get storageclass | grep -E '(NAME|novanas)'

info "Install complete."
