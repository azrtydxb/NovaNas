#!/usr/bin/env bash
# Offline re-signing of a CI-produced RAUC bundle with the NovaNas release key.
#
# Invocation model: run on an air-gapped workstation that holds the 2-of-3
# custody release signing key (HSM preferred; PKCS#11 supported by rauc).
# This script is NOT called by CI.
#
# Example (HSM via PKCS#11):
#   sign-release.sh \
#     --in=novanas-26.07.0.raucb \
#     --cert=/secure/release-cert.pem \
#     --key='pkcs11:token=NovaNasRelease;object=release-key' \
#     --out=novanas-26.07.0.signed.raucb

set -euo pipefail

IN=""
OUT=""
CERT=""
KEY=""
KEYRING=""

usage() {
  cat <<EOF
Usage: $(basename "$0") --in=BUNDLE --cert=CERT --key=KEY [--keyring=KEYRING] --out=BUNDLE
EOF
}

for arg in "$@"; do
  case "$arg" in
    --in=*)      IN="${arg#*=}" ;;
    --out=*)     OUT="${arg#*=}" ;;
    --cert=*)    CERT="${arg#*=}" ;;
    --key=*)     KEY="${arg#*=}" ;;
    --keyring=*) KEYRING="${arg#*=}" ;;
    -h|--help)   usage; exit 0 ;;
    *) echo "unknown arg: $arg" >&2; usage >&2; exit 2 ;;
  esac
done

for v in IN OUT CERT KEY; do
  [[ -n "${!v}" ]] || { echo "--${v,,} required" >&2; exit 2; }
done
[[ -f "$IN" ]] || { echo "missing bundle: $IN" >&2; exit 1; }

command -v rauc >/dev/null 2>&1 || { echo "rauc not installed" >&2; exit 1; }

log() { printf '[sign-release] %s\n' "$*"; }

log "re-signing $IN with $CERT / $KEY"
args=( --cert="$CERT" --key="$KEY" )
[[ -n "$KEYRING" ]] && args+=( --keyring="$KEYRING" )

rauc resign "${args[@]}" "$IN" "$OUT"

log "verifying re-signed bundle"
if [[ -n "$KEYRING" ]]; then
  rauc info --keyring="$KEYRING" "$OUT"
else
  rauc info "$OUT"
fi

log "signed bundle ready at $OUT"
