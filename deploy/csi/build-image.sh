#!/usr/bin/env bash
# Build the nova-csi container image and (optionally) import it into the
# local k3s containerd. Run from anywhere; the script resolves the repo
# root via its own location.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

IMAGE_REPO="${IMAGE_REPO:-novanas/csi}"
IMAGE_TAG="${IMAGE_TAG:-0.1.0-dev}"
IMAGE="${IMAGE_REPO}:${IMAGE_TAG}"

# When IMPORT_K3S=1 is set, pipe the image into k3s's containerd so the
# DaemonSet/Deployment can pull it without a registry. Requires sudo.
IMPORT_K3S="${IMPORT_K3S:-0}"

err()  { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
info() { printf '==> %s\n' "$*"; }

command -v docker >/dev/null 2>&1 || err "docker not found in PATH"

info "Building ${IMAGE}"
info "  context: ${REPO_ROOT}"
info "  dockerfile: deploy/csi/Dockerfile"
docker build \
    --platform linux/amd64 \
    -f "${REPO_ROOT}/deploy/csi/Dockerfile" \
    -t "${IMAGE}" \
    "${REPO_ROOT}"

info "Image built:"
docker images --format '  {{.Repository}}:{{.Tag}}  {{.Size}}' "${IMAGE_REPO}" | grep "${IMAGE_TAG}" || true

if [[ "${IMPORT_K3S}" == "1" ]]; then
    command -v k3s >/dev/null 2>&1 || err "IMPORT_K3S=1 but k3s binary not found"
    info "Importing into k3s containerd (requires sudo)"
    docker save "${IMAGE}" | sudo k3s ctr images import -
    info "Imported. Verify with: sudo k3s ctr images ls | grep ${IMAGE_REPO}"
fi

info "Done. Tag: ${IMAGE}"
