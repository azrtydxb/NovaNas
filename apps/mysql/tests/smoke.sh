#!/usr/bin/env bash
# Smoke test for mysql chart.
# Usage: NAMESPACE=<ns> RELEASE=<release> ./smoke.sh
set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"
RELEASE="${RELEASE:-mysql}"
SVC="${RELEASE}-mysql"
PORT="3306"

echo "[smoke] waiting for deployment/$SVC..."
kubectl -n "$NAMESPACE" rollout status "deployment/$SVC" --timeout=180s

echo "[smoke] probing http://$SVC.$NAMESPACE.svc.cluster.local:$PORT/"
kubectl -n "$NAMESPACE" run smoke-$RANDOM --rm -i --restart=Never --image=curlimages/curl:8.10.1 -- \
  curl -sSf -o /dev/null -m 10 "http://$SVC.$NAMESPACE.svc.cluster.local:$PORT/" \
  || curl -sSf -o /dev/null -m 10 "http://$SVC.$NAMESPACE.svc.cluster.local:$PORT/health" \
  || {
    echo "[smoke] HTTP probe failed; checking TCP..."
    kubectl -n "$NAMESPACE" run smoke-tcp-$RANDOM --rm -i --restart=Never --image=busybox:1.36 -- \
      sh -c "nc -z $SVC $PORT"
  }

echo "[smoke] mysql OK"
