#!/bin/sh
# Provision OpenBao for the NovaNas dev stack: enable transit, create the
# chunk master key, enable KV v2 at novanas/, install dev policies.
#
# Safe to re-run — all commands are idempotent (errors for "already exists"
# are tolerated).

set -eu

: "${BAO_ADDR:=http://openbao:8200}"
: "${BAO_TOKEN:=dev-token}"
export BAO_ADDR BAO_TOKEN

echo "[openbao-init] waiting for openbao at ${BAO_ADDR}"
i=0
until bao status >/dev/null 2>&1; do
  i=$((i + 1))
  if [ "$i" -gt 60 ]; then
    echo "[openbao-init] openbao did not become ready in time" >&2
    exit 1
  fi
  sleep 1
done

echo "[openbao-init] enabling transit"
bao secrets enable transit 2>/dev/null || echo "[openbao-init] transit already enabled"

echo "[openbao-init] creating chunk master key"
bao write -f transit/keys/novanas/chunk-master type=aes256-gcm96 || true

echo "[openbao-init] enabling KV v2 at novanas/"
bao secrets enable -path=novanas -version=2 kv 2>/dev/null || echo "[openbao-init] kv already enabled at novanas/"

echo "[openbao-init] writing dev policies"
cat <<'EOF' | bao policy write novanas-api -
path "transit/encrypt/novanas/chunk-master" { capabilities = ["update"] }
path "transit/decrypt/novanas/chunk-master" { capabilities = ["update"] }
path "novanas/data/*"                       { capabilities = ["create","read","update","delete","list"] }
path "novanas/metadata/*"                   { capabilities = ["read","list","delete"] }
EOF

echo "[openbao-init] done"
