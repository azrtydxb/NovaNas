#!/usr/bin/env bash
# Re-build the RAUC bundle with the release-signing key+cert supplied as
# GitHub Actions secrets (beta). The unsigned dev bundle produced by the
# os-build workflow is discarded; this script writes a signed bundle to
# the same output path.
#
# TODO(GA): replace Actions-secret signing with Cloud KMS (AWS KMS /
# Azure Key Vault / GCP KMS). The RAUC tooling gained pkcs11 support,
# so once we have a KMS-backed PKCS#11 module we just swap --cert/--key
# for the pkcs11: URI. See docs/developer/ci.md for the migration plan.
set -eo pipefail

: "${VERSION:?VERSION must be set (tag, e.g. v1.2.3)}"
: "${RAUC_SIGNING_KEY:?RAUC_SIGNING_KEY secret missing}"
: "${RAUC_SIGNING_CERT:?RAUC_SIGNING_CERT secret missing}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="$ROOT/os/build/out"
KEY_FILE="$(mktemp)"
CERT_FILE="$(mktemp)"
trap 'shred -u "$KEY_FILE" "$CERT_FILE" 2>/dev/null || rm -f "$KEY_FILE" "$CERT_FILE"' EXIT

printf '%s' "$RAUC_SIGNING_KEY" > "$KEY_FILE"
printf '%s' "$RAUC_SIGNING_CERT" > "$CERT_FILE"

BUNDLE="$OUT/novanas-${VERSION#v}.raucb"
ROOTFS="$OUT/rootfs-${VERSION#v}.img"
BOOT="$OUT/boot-${VERSION#v}.img"

if [ ! -f "$ROOTFS" ] || [ ! -f "$BOOT" ]; then
  echo "rauc-sign-release.sh: expected rootfs/boot images under $OUT" >&2
  ls -la "$OUT" >&2 || true
  exit 1
fi

rm -f "$BUNDLE"
"$ROOT/os/build/build-rauc-bundle.sh" \
  --version="${VERSION#v}" --channel="stable" \
  --rootfs="$ROOTFS" --boot="$BOOT" \
  --manifest="$ROOT/os/rauc/manifest.raucm" \
  --cert="$CERT_FILE" --key="$KEY_FILE" \
  --out="$BUNDLE"

echo "Signed RAUC bundle: $BUNDLE"
