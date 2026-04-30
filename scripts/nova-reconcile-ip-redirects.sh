#!/bin/bash
# nova-reconcile-ip-redirects: ensure the current LAN IP is in the
# Keycloak nova-api client's redirectUris and webOrigins so users
# hitting the SPA via raw IP (rather than novanas.local) can still
# complete the OIDC dance. Idempotent — safe to re-run.
#
# Triggered by: nova-reconcile-ip-redirects.service (oneshot, after
# keycloak.service). Wait for Keycloak to actually accept logins
# before bailing.
set -euo pipefail

KC_URL="${KC_URL:-https://localhost:8443}"
KC_USER="${KC_USER:-admin}"
KC_PASS="${KC_PASS:-adminpw}"
REALM="${REALM:-novanas}"
CLIENT_ID="${CLIENT_ID:-nova-api}"
SPA_PORT="${SPA_PORT:-8444}"
LOG=/var/log/nova-reconcile-ip-redirects.log

log() { echo "$(date -Iseconds) $*" | tee -a "$LOG"; }

# Detect the primary IPv4 — the source the kernel uses to reach the
# default-route gateway. Falls back to the first non-loopback IPv4 if
# `ip route get` is unavailable.
ip="$(ip -4 -o route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit}}')"
if [ -z "$ip" ]; then
  ip="$(ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | head -1)"
fi
if [ -z "$ip" ]; then
  log "FATAL: could not detect primary IPv4"
  exit 1
fi
log "primary IPv4 = $ip"

# Ensure novanas.local resolves locally on the box itself. Go's
# pure-Go resolver (CGO_ENABLED=0 binaries) does not consult NSS, so
# libnss-mdns can't help here — but it does read /etc/hosts. Without
# this entry nova-api fails JWKS discovery against its own Keycloak
# with "no such host" and rejects every JWT.
if ! grep -qE "^[[:space:]]*127\.0\.0\.1[[:space:]]+.*\\bnovanas\\.local\\b" /etc/hosts; then
  echo "127.0.0.1 novanas.local NovaNAS.local" >> /etc/hosts
  log "appended /etc/hosts entry for novanas.local"
fi

# Trust the NovaNAS Local CA system-wide so Go binaries (CGO_ENABLED=0
# builds use only the system CA bundle) can verify
# https://novanas.local:8443 — needed for nova-api to fetch JWKS from
# its own Keycloak. Trusting the CA (not the leaf) means leaf cert
# rotations re-sign without a re-trust step. Also clean up any older
# leaf-trust entry from previous deploys.
rm -f /usr/local/share/ca-certificates/novanas-dev.crt
CA_SRC=/etc/nova-ca/ca.crt
CA_DST=/usr/local/share/ca-certificates/novanas-ca.crt
if [ -f "$CA_SRC" ] && ! cmp -s "$CA_SRC" "$CA_DST" 2>/dev/null; then
  install -m 0644 "$CA_SRC" "$CA_DST"
  update-ca-certificates >/dev/null 2>&1 || true
  log "installed NovaNAS Local CA into system trust store"
fi

# Wait for Keycloak (up to 60s) — restart-on-failure may stall it.
for i in $(seq 1 30); do
  if curl -sk -o /dev/null -w "%{http_code}" "$KC_URL/realms/master/protocol/openid-connect/token" \
       -d grant_type=password -d client_id=admin-cli -d "username=$KC_USER" -d "password=$KC_PASS" \
       | grep -q "200"; then
    break
  fi
  sleep 2
done

token="$(curl -sk "$KC_URL/realms/master/protocol/openid-connect/token" \
  -d grant_type=password -d client_id=admin-cli \
  -d "username=$KC_USER" -d "password=$KC_PASS" \
  | python3 -c 'import json,sys;print(json.load(sys.stdin).get("access_token",""))')"
if [ -z "$token" ]; then
  log "FATAL: could not obtain admin token"
  exit 1
fi

# Look up the client's UUID by clientId.
client_uuid="$(curl -sk -H "Authorization: Bearer $token" \
  "$KC_URL/admin/realms/$REALM/clients?clientId=$CLIENT_ID" \
  | python3 -c 'import json,sys;a=json.load(sys.stdin);print(a[0]["id"] if a else "")')"
if [ -z "$client_uuid" ]; then
  log "FATAL: client $CLIENT_ID not found in realm $REALM"
  exit 1
fi

# Read current client config.
cfg="$(curl -sk -H "Authorization: Bearer $token" \
  "$KC_URL/admin/realms/$REALM/clients/$client_uuid")"

new_cfg="$(CFG="$cfg" python3 -c '
import json, os, sys
ip, port = sys.argv[1], sys.argv[2]
cfg = json.loads(os.environ["CFG"])
redir = cfg.get("redirectUris") or []
origs = cfg.get("webOrigins") or []
want_redir = f"https://{ip}:{port}/*"
want_orig  = f"https://{ip}:{port}"
changed = False
if want_redir not in redir:
    redir.append(want_redir); changed = True
if want_orig not in origs:
    origs.append(want_orig); changed = True
if changed:
    cfg["redirectUris"] = redir
    cfg["webOrigins"]   = origs
    print(json.dumps(cfg))
' "$ip" "$SPA_PORT")"

if [ -z "$new_cfg" ]; then
  log "no change (current IP already registered)"
  exit 0
fi

curl -sk -X PUT "$KC_URL/admin/realms/$REALM/clients/$client_uuid" \
  -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
  --data "$new_cfg" -w "%{http_code}\n" -o /tmp/.nova-reconcile-resp >> "$LOG"
log "updated client $CLIENT_ID with IP $ip"
