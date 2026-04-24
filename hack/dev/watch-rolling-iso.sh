#!/usr/bin/env bash
# Watch the rolling-dev GitHub release and keep a local ISO copy in sync.
# Intended for the developer MBP so a local HTTP server (python3 -m
# http.server) can hand the latest ISO to an IP-KVM's virtual-media URL.
#
# Usage:
#   hack/dev/watch-rolling-iso.sh [--dir <target>] [--interval <secs>] [--serve-port <port>]
#
# Defaults: target=~/Downloads/, interval=60s, serve-port=0 (no server)
#
# Requires gh CLI + a valid `gh auth` session that can read the private
# repo's releases.

set -euo pipefail

TARGET="$HOME/Downloads"
INTERVAL=60
SERVE_PORT=0
REPO="azrtydxb/NovaNas"
TAG="rolling-dev"
ASSET="novanas-dev.iso"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir)        TARGET="$2"; shift 2 ;;
    --interval)   INTERVAL="$2"; shift 2 ;;
    --serve-port) SERVE_PORT="$2"; shift 2 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

mkdir -p "$TARGET"
LOCAL="$TARGET/$ASSET"

log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }

serve_pid=""
if [[ "$SERVE_PORT" != 0 ]]; then
  cd "$TARGET"
  log "starting HTTP server on 0.0.0.0:$SERVE_PORT serving $TARGET"
  python3 -m http.server "$SERVE_PORT" --bind 0.0.0.0 &
  serve_pid=$!
  trap 'kill $serve_pid 2>/dev/null || true' EXIT
fi

last_asset_id=""
while :; do
  if info=$(gh api "repos/$REPO/releases/tags/$TAG" 2>/dev/null); then
    asset_id=$(jq -r --arg n "$ASSET" '.assets[] | select(.name==$n) | .id // empty' <<<"$info")
    updated=$(jq -r --arg n "$ASSET" '.assets[] | select(.name==$n) | .updated_at // empty' <<<"$info")
    size=$(jq -r --arg n "$ASSET" '.assets[] | select(.name==$n) | .size // 0' <<<"$info")
    if [[ -n "$asset_id" && "$asset_id" != "$last_asset_id" ]]; then
      log "new asset: id=$asset_id size=$size updated=$updated — downloading"
      gh release download "$TAG" -R "$REPO" -p "$ASSET" -D "$TARGET" --clobber 2>&1 | sed 's/^/  /'
      ls -lh "$LOCAL" 2>/dev/null | sed 's/^/  /'
      last_asset_id="$asset_id"
    fi
  else
    log "gh api failed (auth? network?) — retrying in ${INTERVAL}s"
  fi
  sleep "$INTERVAL"
done
