#!/usr/bin/env bash
# tests/test-download.sh
set -e
cd "$(dirname "$0")/.."
NETINST_URL="https://example.invalid/nope.iso" \
  NETINST_SHA256="0000" \
  ./build-installer-iso.sh 2>&1 | grep -q "checksum mismatch" \
  && echo "PASS" || { echo "FAIL: expected checksum mismatch error"; exit 1; }
