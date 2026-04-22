#!/usr/bin/env bash
# NovaNas post-update health check.
# Runs every boot; on a RAUC-pending slot this is the gate that commits the
# upgrade. Exits 0 to keep the slot (RAUC mark-good is invoked here). Any
# non-zero exit leaves the slot unmarked; GRUB countboot handles rollback.

set -euo pipefail

KUBECONFIG="/etc/rancher/k3s/k3s.yaml"
NAMESPACE="novanas-system"
TIMEOUT_SEC=600

log() { printf '[healthcheck] %s\n' "$*"; }

slot_is_pending() {
  # Only act on a slot that RAUC considers booted-but-not-yet-good.
  if ! command -v rauc >/dev/null 2>&1; then
    return 1
  fi
  rauc status --output-format=shell 2>/dev/null | grep -q '^RAUC_SLOT_STATE=.*booted' || return 1
  ! rauc status --output-format=shell 2>/dev/null | grep -q '^RAUC_SLOT_BOOT_STATUS_[A-Z0-9]*=good$'
}

wait_for_api() {
  log "waiting for k3s API"
  local deadline=$(( $(date +%s) + TIMEOUT_SEC ))
  while (( $(date +%s) < deadline )); do
    if KUBECONFIG="$KUBECONFIG" kubectl --request-timeout=5s get --raw='/readyz' 2>/dev/null | grep -q '^ok$'; then
      return 0
    fi
    sleep 5
  done
  log "k3s API did not become ready"
  return 1
}

wait_for_api_pod() {
  log "waiting for novanas-api pod Ready"
  if ! KUBECONFIG="$KUBECONFIG" kubectl -n "$NAMESPACE" wait \
      --for=condition=Ready pod -l app.kubernetes.io/name=novanas-api \
      --timeout="${TIMEOUT_SEC}s"; then
    log "novanas-api pod did not become Ready"
    return 1
  fi
}

no_crashloops() {
  log "checking for CrashLoopBackOff in $NAMESPACE"
  local bad
  bad=$(KUBECONFIG="$KUBECONFIG" kubectl -n "$NAMESPACE" get pods \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.containerStatuses[*].state.waiting.reason}{"\n"}{end}' \
    | grep -c CrashLoopBackOff || true)
  if [[ "$bad" -gt 0 ]]; then
    log "$bad pod(s) in CrashLoopBackOff"
    return 1
  fi
  return 0
}

mark_good_if_pending() {
  if slot_is_pending; then
    log "marking current slot good"
    rauc status mark-good booted
  else
    log "slot is already good or RAUC unavailable; nothing to mark"
  fi
}

main() {
  wait_for_api
  wait_for_api_pod
  no_crashloops
  mark_good_if_pending
  log "health check passed"
}

main "$@"
