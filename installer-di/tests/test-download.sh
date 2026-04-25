#!/usr/bin/env bash
# Test: download_netinst() detects sha256 mismatch on cached file.
# Pre-populates the cache with a known file, then runs the build script
# with a deliberately wrong SHA. Skips the curl download (cache hit) so we
# hit the sha verification path directly. We also stub xorriso/gunzip in
# PATH so the dependency check passes.
set -eu
cd "$(dirname "$0")/.."

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

# Stub the binaries the build script's dependency check requires.
mkdir "$WORK/bin"
for cmd in xorriso gunzip; do
  printf '#!/bin/sh\nexit 0\n' > "$WORK/bin/$cmd"
  chmod +x "$WORK/bin/$cmd"
done
export PATH="$WORK/bin:$PATH"

# Pre-populate cache so download_netinst skips curl and goes to sha check.
TEST_URL="https://example.invalid/test.iso"
mkdir -p netinst-cache
echo "fake iso content" > netinst-cache/test.iso

# Run the build script with a wrong SHA. Capture all output.
output=$(NETINST_URL="$TEST_URL" NETINST_SHA256="0000bad" \
  WORK_DIR="$WORK/work" OUT_ISO="$WORK/out.iso" \
  ./build-installer-iso.sh 2>&1 || true)

# Cleanup the seed file regardless of test outcome.
rm -f netinst-cache/test.iso

if printf '%s\n' "$output" | grep -q "checksum mismatch"; then
  echo "PASS"
else
  printf '%s\n' "$output" >&2
  echo "FAIL: expected 'checksum mismatch' in output" >&2
  exit 1
fi
