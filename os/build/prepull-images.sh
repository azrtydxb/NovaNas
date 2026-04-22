#!/usr/bin/env bash
# Pre-pull every container image referenced by the NovaNas umbrella Helm chart
# into /var/lib/rancher/k3s/agent/images/ so first-boot on a freshly-installed
# appliance works with zero network access.
#
# Input: image manifest at arg $2 — a flat text file, one image ref per line,
# comments starting with '#' ignored.
# Output: a single `images.tar.zst` in arg $1 ready to be loaded by k3s's
# auto-import mechanism.

set -euo pipefail

OUT_DIR="${1:-}"
MANIFEST="${2:-}"

[[ -n "$OUT_DIR" && -n "$MANIFEST" ]] || {
  echo "Usage: $(basename "$0") <out-dir> <image-manifest>" >&2
  exit 2
}

[[ -f "$MANIFEST" ]] || { echo "manifest not found: $MANIFEST" >&2; exit 1; }

mkdir -p "$OUT_DIR"

log() { printf '[prepull] %s\n' "$*"; }

# Prefer skopeo (archive-friendly); fall back to crane.
PULLER=""
if command -v skopeo >/dev/null 2>&1; then
  PULLER="skopeo"
elif command -v crane >/dev/null 2>&1; then
  PULLER="crane"
else
  log "neither skopeo nor crane installed; SKIPPING image pre-pull."
  log "First boot on this image will need to hit the network to pull images."
  exit 0
fi

log "using $PULLER"

STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

mapfile -t IMAGES < <(grep -v '^\s*#' "$MANIFEST" | grep -v '^\s*$')

for img in "${IMAGES[@]}"; do
  safe=$(echo "$img" | tr '/:@' '___')
  tarball="$STAGE/${safe}.tar"
  log "pull $img"
  case "$PULLER" in
    skopeo) skopeo copy --all "docker://$img" "docker-archive:$tarball:$img" ;;
    crane)  crane pull --format=oci "$img" "$tarball" ;;
  esac
done

log "bundling to images.tar.zst"
tar -cf - -C "$STAGE" . | zstd -T0 -19 -o "$OUT_DIR/images.tar.zst"
log "wrote $OUT_DIR/images.tar.zst"
