#!/usr/bin/env bash
# NovaNas first-boot initialization.
# Runs once per OS install (guarded by /var/lib/novanas/.firstboot-done).
#
# Responsibilities:
#   1. Prepare the persistent partition layout.
#   2. Wait for k3s to be reachable.
#   3. Install the NovaNas umbrella Helm chart.
#   4. Drop the sentinel so we don't run again.

set -euo pipefail

SENTINEL="/var/lib/novanas/.firstboot-done"
PERSISTENT="/mnt/persistent"
HELM_CHART_DIR="/opt/novanas/helm"
KUBECONFIG="/etc/rancher/k3s/k3s.yaml"
NAMESPACE="novanas-system"

log() { printf '[firstboot] %s\n' "$*"; }
die() { printf '[firstboot] FATAL: %s\n' "$*" >&2; exit 1; }

ensure_persistent_layout() {
  log "preparing $PERSISTENT layout"
  install -d -m 0755 \
    "$PERSISTENT"/etc \
    "$PERSISTENT"/var \
    "$PERSISTENT"/home \
    "$PERSISTENT"/postgres \
    "$PERSISTENT"/openbao \
    "$PERSISTENT"/k3s \
    "$PERSISTENT"/rauc \
    "$PERSISTENT"/logs \
    "$PERSISTENT"/novanas
  install -d -m 0700 "$PERSISTENT"/openbao
}

wait_for_k3s() {
  log "waiting for k3s API to come up"
  local deadline=$(( $(date +%s) + 300 ))
  while (( $(date +%s) < deadline )); do
    if KUBECONFIG="$KUBECONFIG" kubectl --request-timeout=5s get --raw='/readyz' 2>/dev/null | grep -q '^ok$'; then
      log "k3s ready"
      return 0
    fi
    sleep 3
  done
  die "k3s did not become ready within 5 min"
}

install_helm_chart() {
  if [[ ! -d "$HELM_CHART_DIR" ]]; then
    log "no chart at $HELM_CHART_DIR; skipping helm install (dev image?)"
    return 0
  fi
  log "installing NovaNas umbrella chart"
  KUBECONFIG="$KUBECONFIG" helm upgrade --install novanas "$HELM_CHART_DIR" \
    --namespace "$NAMESPACE" \
    --create-namespace \
    --wait --timeout 20m \
    --set global.version="$(cat /etc/novanas/version)" \
    --set global.channel="$(cat /etc/novanas/channel)"
}

mark_done() {
  install -d -m 0755 "$(dirname "$SENTINEL")"
  date --iso-8601=seconds > "$SENTINEL"
  log "first boot complete"
}

main() {
  if [[ -f "$SENTINEL" ]]; then
    log "sentinel exists; nothing to do"
    exit 0
  fi
  ensure_persistent_layout
  wait_for_k3s
  install_helm_chart
  mark_done
}

main "$@"
