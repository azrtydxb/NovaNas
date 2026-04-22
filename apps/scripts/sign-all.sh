#!/usr/bin/env bash
# Cosign-sign every chart in the catalog after it has been pushed to OCI.
# Requires COSIGN_KEY or keyless (OIDC) env.
set -eo pipefail

REGISTRY="${REGISTRY:-ghcr.io/azrtydxb/novanas-apps}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

command -v cosign >/dev/null || { echo "cosign not found"; exit 1; }

for chart in "$ROOT"/*/chart; do
  [ -d "$chart" ] || continue
  name="$(basename "$(dirname "$chart")")"
  version="$(awk '/^version:/ {print $2; exit}' "$chart/Chart.yaml")"
  ref="$REGISTRY/$name:$version"
  echo "==> Signing $ref"
  cosign sign --yes "$ref"
done

echo "All charts signed."
