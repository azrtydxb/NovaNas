#!/usr/bin/env bash
# Package and push every chart in the catalog to an OCI registry.
# Does NOT sign — run sign-all.sh after this to attach cosign signatures.
set -eo pipefail

REGISTRY="${REGISTRY:-ghcr.io/azrtydxb/novanas-apps}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

for chart in "$ROOT"/*/chart; do
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
