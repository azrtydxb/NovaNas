#!/usr/bin/env bash
# Lint every chart in the catalog.
set -eo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FAIL=0

for chart in "$ROOT"/*/chart; do
  [ -d "$chart" ] || continue
  name="$(basename "$(dirname "$chart")")"
  echo "==> Linting $name"
  if ! helm lint "$chart"; then
    FAIL=$((FAIL + 1))
    echo "FAIL: $name"
  fi
done

if [ "$FAIL" -ne 0 ]; then
  echo "$FAIL chart(s) failed lint"
  exit 1
fi
echo "All charts passed helm lint."
