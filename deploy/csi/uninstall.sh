#!/usr/bin/env bash
# Uninstall the NovaNAS CSI driver. Does NOT touch PersistentVolumes:
# orphan PVs may still reference real datasets/zvols on the host, and
# automated deletion would risk data loss. Operator must clean those up.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFESTS="${SCRIPT_DIR}/manifests"

err()  { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
info() { printf '==> %s\n' "$*"; }
warn() { printf 'WARNING: %s\n' "$*" >&2; }

command -v kubectl >/dev/null 2>&1 || err "kubectl not found in PATH"

# Surface dangling PVs before tearing down the driver. Once the
# provisioner is gone, the PVs cannot be deleted via Kubernetes and have
# to be cleaned up manually (kubectl delete pv --force, then destroy the
# underlying ZFS dataset).
PV_COUNT=$(kubectl get pv -o json 2>/dev/null \
    | grep -c '"driver": "csi.novanas.io"' || true)
if [[ "${PV_COUNT}" -gt 0 ]]; then
    warn "${PV_COUNT} PersistentVolume(s) still reference csi.novanas.io."
    warn "These will become orphans after uninstall. List them with:"
    warn "  kubectl get pv -o json | jq -r '.items[] | select(.spec.csi.driver==\"csi.novanas.io\") | .metadata.name'"
    warn "Press Ctrl-C in the next 5s to abort."
    sleep 5
fi

# Reverse order. Don't fail if a manifest is already gone.
ORDER=(
    "60-node.yaml"
    "50-controller.yaml"
    "40-storageclass-block.yaml"
    "40-storageclass-fs.yaml"
    "30-volumesnapshotclass.yaml"
    "20-csidriver.yaml"
    "10-rbac.yaml"
    "00-namespace.yaml"
)

for f in "${ORDER[@]}"; do
    path="${MANIFESTS}/${f}"
    [[ -f "${path}" ]] || continue
    info "Deleting ${f}"
    kubectl delete -f "${path}" --ignore-not-found=true
done

info "Uninstall complete. The Secret nova-csi-auth (if any) was deleted with the namespace."
info "On the storage host, leftover bind mounts under /var/lib/kubelet/pods/*/volumes/kubernetes.io~csi/* may need manual umount."
