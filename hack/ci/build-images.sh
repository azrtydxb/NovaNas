#!/usr/bin/env bash
# Build and push all NovaNas container images for a release tag.
# Iterates through every packages/*/Dockerfile and storage/*/Dockerfile*
# matching the known image set, tagging :${VERSION} (e.g. v1.2.3).
#
# No-op for images whose Dockerfile is not present (so partial releases
# from an early-stage tree still work).
set -eo pipefail

REGISTRY="${REGISTRY:-ghcr.io/azrtydxb/novanas}"
VERSION="${VERSION:?VERSION must be set (e.g. v1.2.3)}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# image -> Dockerfile path. Build context is always ROOT — every
# Dockerfile in this repo COPYs go.work / pnpm-lock from the monorepo
# root. Skip silently if the Dockerfile is missing (so partial-tree
# tags still work in early development).
declare -a images=(
  "api:packages/api/Dockerfile"
  "ui:packages/ui/Dockerfile"
  "operators:packages/operators/Dockerfile"
  "disk-agent:packages/operators/cmd/disk-agent/Dockerfile"
  "storage-meta:storage/cmd/meta/Dockerfile"
  "storage-agent:storage/cmd/agent/Dockerfile"
  "storage-controller:storage/cmd/controller/Dockerfile"
  "storage-scheduler:storage/cmd/scheduler/Dockerfile"
  "storage-webhook:storage/cmd/webhook/Dockerfile"
  "storage-csi:storage/cmd/csi/Dockerfile"
  "storage-s3gw:storage/cmd/s3gw/Dockerfile"
  "storage-dataplane:storage/dataplane/Dockerfile.build"
)

for entry in "${images[@]}"; do
  name="${entry%%:*}"
  dockerfile_rel="${entry#*:}"
  dockerfile="$ROOT/$dockerfile_rel"
  if [ ! -f "$dockerfile" ]; then
    echo "skip: $name (no Dockerfile at $dockerfile_rel)"
    continue
  fi
  tag="$REGISTRY/$name:$VERSION"
  echo "==> build+push $tag  (Dockerfile: $dockerfile_rel)"
  docker build -f "$dockerfile" -t "$tag" "$ROOT"
  docker push "$tag"
done

echo "All images built+pushed for $VERSION."
