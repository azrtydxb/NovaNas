#!/usr/bin/env bash
# Package and push every app chart in apps/*/chart to the OCI registry.
# Defaults to ghcr.io/azrtydxb/novanas-apps; override with $REGISTRY.
# Sibling to publish.sh (kept for backwards compatibility).
set -eo pipefail

REGISTRY="${REGISTRY:-ghcr.io/azrtydxb/novanas-apps}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

shopt -s nullglob
charts=("$ROOT"/*/chart)
if [ ${#charts[@]} -eq 0 ]; then
  echo "publish-all.sh: no charts found under $ROOT/*/chart" >&2
  exit 0
fi

for chart in "${charts[@]}"; do
  [ -d "$chart" ] || continue
  name="$(basename "$(dirname "$chart")")"
  echo "==> Packaging $name"
  helm package "$chart" --destination "$WORK"
done

for tgz in "$WORK"/*.tgz; do
  echo "==> Pushing $(basename "$tgz") -> oci://$REGISTRY"
  helm push "$tgz" "oci://$REGISTRY"
done

echo "All charts published to $REGISTRY."
