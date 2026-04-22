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

# image -> build context (Dockerfile assumed at <context>/Dockerfile unless overridden)
declare -a images=(
  "api:packages/api"
  "ui:packages/ui"
  "operators:packages/operators"
  "storage-meta:storage/meta"
  "storage-agent:storage/agent"
)

for entry in "${images[@]}"; do
  name="${entry%%:*}"
  ctx="${entry#*:}"
  dockerfile="$ROOT/$ctx/Dockerfile"
  if [ ! -f "$dockerfile" ]; then
    # Fall back to dataplane's Dockerfile.build for storage-agent variant.
    if [ "$name" = "storage-agent" ] && [ -f "$ROOT/storage/dataplane/Dockerfile.build" ]; then
      ctx="storage/dataplane"
      dockerfile="$ROOT/$ctx/Dockerfile.build"
    else
      echo "skip: $name (no Dockerfile at $dockerfile)"
      continue
    fi
  fi
  tag="$REGISTRY/$name:$VERSION"
  echo "==> build+push $tag  (context: $ctx)"
  docker build -f "$dockerfile" -t "$tag" "$ROOT/$ctx"
  docker push "$tag"
done

echo "All images built+pushed for $VERSION."
