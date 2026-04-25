#!/usr/bin/env bash
# Thin wrapper that invokes the d-i-based ISO builder. The old mkosi-live
# pipeline was removed in favor of a debian-installer ISO repack — see
# installer-di/build-installer-iso.sh for the actual logic.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
exec "$REPO_ROOT/installer-di/build-installer-iso.sh" "$@"
