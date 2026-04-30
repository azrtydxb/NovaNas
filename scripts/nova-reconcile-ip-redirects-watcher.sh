#!/bin/bash
# Long-running watcher: triggers nova-reconcile-ip-redirects whenever
# any IPv4 address change is observed on a global-scope interface.
# Catches DHCP renews, GUI-driven Network app edits, and manual
# `ip addr` commands alike.
#
# `ip monitor address` emits one line per add/del event. We debounce
# by sleeping briefly after each burst so we run the reconciler once
# even when an interface flaps several times in a row.
set -euo pipefail

BIN=/usr/local/bin/nova-reconcile-ip-redirects
DEBOUNCE=3

while true; do
  if ! ip -4 monitor address 2>&1 | while read -r line; do
    # Filter scope-global only; skip link-local and loopback.
    case "$line" in
      *scope\ global*) ;;
      *) continue ;;
    esac
    # Drain bursts: read additional lines for DEBOUNCE seconds, then act once.
    while read -t "$DEBOUNCE" -r _ ; do :; done
    "$BIN" || true
  done; then
    sleep 5  # ip monitor died; brief backoff then re-arm
  fi
done
