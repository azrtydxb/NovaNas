#!/usr/bin/env bash
# Create (or refresh) one Keycloak client per oauth2-proxy-protected
# observability service: prometheus, alertmanager, loki.
#
# Each client is:
#   - confidential auth-code flow
#   - has redirect URI <SERVICE_URL>/oauth2/callback
#   - has an audience mapper so the client_id appears in token aud
#
# A fresh cookie-secret (32 random bytes, base64-encoded) is generated
# per service. The script emits a JSON object to stdout listing every
# client and its secrets:
#
# {
#   "clients": [
#     {"service":"prometheus","clientId":"oauth2-proxy-prometheus",
#      "clientSecret":"...","cookieSecret":"..."},
#     ...
#   ]
# }
#
# Operator usage:
#
#   ./create-oauth2-proxy-clients.sh > /tmp/o2p.json
#   for svc in prometheus alertmanager loki; do
#     CS=$(jq -r --arg s "$svc" '.clients[]|select(.service==$s)|.clientSecret' /tmp/o2p.json)
#     CK=$(jq -r --arg s "$svc" '.clients[]|select(.service==$s)|.cookieSecret' /tmp/o2p.json)
#     printf '%s' "$CS" | sudo install -m 0400 -o oauth2-proxy -g oauth2-proxy \
#       /dev/stdin "/etc/oauth2-proxy/${svc}-client-secret"
#     printf '%s' "$CK" | sudo install -m 0400 -o oauth2-proxy -g oauth2-proxy \
#       /dev/stdin "/etc/oauth2-proxy/${svc}-cookie-secret"
#   done
#
# Required env (or pass --kc-* flags):
#   KC_URL        Keycloak base URL (e.g. https://192.168.10.204:8443)
#   KC_REALM      Target realm (default: novanas)
#   KC_ADMIN_USER Keycloak admin user (default: admin)
#   KC_ADMIN_PASS Keycloak admin password
#   PUBLIC_HOST   Hostname browsers reach the proxies at (default: novanas.local)

set -euo pipefail

KC_URL="${KC_URL:-}"
KC_REALM="${KC_REALM:-novanas}"
KC_ADMIN_USER="${KC_ADMIN_USER:-admin}"
KC_ADMIN_PASS="${KC_ADMIN_PASS:-}"
KCADM="${KCADM:-kcadm.sh}"
PUBLIC_HOST="${PUBLIC_HOST:-novanas.local}"

usage() {
    cat <<EOF >&2
usage: $0 [--kc-url URL] [--realm REALM] [--admin-user USER] [--admin-pass PASS]
          [--public-host HOST]

Creates/updates oauth2-proxy-{prometheus,alertmanager,loki} clients in
Keycloak realm '$KC_REALM' and prints client + cookie secrets as JSON.
EOF
    exit 2
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --kc-url) KC_URL="$2"; shift 2 ;;
        --realm) KC_REALM="$2"; shift 2 ;;
        --admin-user) KC_ADMIN_USER="$2"; shift 2 ;;
        --admin-pass) KC_ADMIN_PASS="$2"; shift 2 ;;
        --public-host) PUBLIC_HOST="$2"; shift 2 ;;
        -h|--help) usage ;;
        *) echo "unknown arg: $1" >&2; usage ;;
    esac
done

[[ -n "$KC_URL" ]] || { echo "ERROR: KC_URL or --kc-url required" >&2; exit 2; }
[[ -n "$KC_ADMIN_PASS" ]] || { echo "ERROR: KC_ADMIN_PASS or --admin-pass required" >&2; exit 2; }
command -v "$KCADM" >/dev/null 2>&1 || { echo "ERROR: kcadm.sh ($KCADM) not found in PATH" >&2; exit 2; }
command -v jq >/dev/null 2>&1 || { echo "ERROR: jq required" >&2; exit 2; }
command -v openssl >/dev/null 2>&1 || { echo "ERROR: openssl required" >&2; exit 2; }

CFGDIR="$(mktemp -d)"
trap 'rm -rf "$CFGDIR"' EXIT
# kcadm.sh on recent Keycloak only accepts --config AFTER the subcommand,
# so we isolate per-invocation state via HOME instead. CFGDIR is a tmpdir
# the trap above cleans up.
export HOME="$CFGDIR"
KC="$KCADM"

$KC config credentials \
    --server "$KC_URL" \
    --realm master \
    --user "$KC_ADMIN_USER" \
    --password "$KC_ADMIN_PASS" >/dev/null

# service -> external HTTPS port (oauth2-proxy listener)
declare -A PORTS=(
    [prometheus]=9091
    [alertmanager]=9094
    [loki]=3101
)

create_or_update_client() {
    local service="$1"
    local port="$2"
    local client_id="oauth2-proxy-${service}"
    local public_url="https://${PUBLIC_HOST}:${port}"
    local redirect_uri="${public_url}/oauth2/callback"

    local existing_id
    existing_id="$($KC get clients -r "$KC_REALM" -q "clientId=$client_id" --fields id --format csv --noquotes 2>/dev/null | head -n1 || true)"

    local payload
    payload=$(cat <<JSON
{
  "clientId": "$client_id",
  "name": "NovaNAS oauth2-proxy ($service)",
  "description": "Auth-code OIDC client used by oauth2-proxy in front of $service",
  "enabled": true,
  "protocol": "openid-connect",
  "publicClient": false,
  "clientAuthenticatorType": "client-secret",
  "standardFlowEnabled": true,
  "directAccessGrantsEnabled": false,
  "implicitFlowEnabled": false,
  "serviceAccountsEnabled": false,
  "frontchannelLogout": true,
  "rootUrl": "$public_url",
  "baseUrl": "$public_url",
  "redirectUris": ["$redirect_uri"],
  "webOrigins": ["$public_url"],
  "attributes": {
    "post.logout.redirect.uris": "$public_url/*",
    "pkce.code.challenge.method": "S256"
  },
  "defaultClientScopes": ["web-origins", "profile", "roles", "email"],
  "optionalClientScopes": ["address", "phone", "offline_access", "microprofile-jwt"]
}
JSON
)

    if [[ -z "$existing_id" ]]; then
        echo "Creating client '$client_id'" >&2
        $KC create clients -r "$KC_REALM" -f - <<<"$payload" >/dev/null
        existing_id="$($KC get clients -r "$KC_REALM" -q "clientId=$client_id" --fields id --format csv --noquotes 2>/dev/null | head -n1)"
    else
        echo "Updating existing client '$client_id' (id=$existing_id)" >&2
        $KC update "clients/$existing_id" -r "$KC_REALM" -f - <<<"$payload"
    fi

    # Audience mapper.
    local aud_payload
    aud_payload=$(cat <<JSON
{
  "name": "${service}-audience",
  "protocol": "openid-connect",
  "protocolMapper": "oidc-audience-mapper",
  "config": {
    "included.client.audience": "$client_id",
    "id.token.claim": "true",
    "access.token.claim": "true"
  }
}
JSON
)
    local existing_mapper
    existing_mapper="$($KC get "clients/$existing_id/protocol-mappers/models" -r "$KC_REALM" --fields id,name --format csv --noquotes 2>/dev/null | awk -F, -v n="${service}-audience" '$2==n{print $1}' | head -n1 || true)"
    if [[ -z "$existing_mapper" ]]; then
        $KC create "clients/$existing_id/protocol-mappers/models" -r "$KC_REALM" -f - <<<"$aud_payload" >/dev/null
    fi

    # Rotate secret.
    $KC create "clients/$existing_id/client-secret" -r "$KC_REALM" >/dev/null 2>&1 || true
    local secret
    secret="$($KC get "clients/$existing_id/client-secret" -r "$KC_REALM" --fields value --format csv --noquotes 2>/dev/null | head -n1)"
    if [[ -z "$secret" || "$secret" == "null" ]]; then
        echo "ERROR: failed to read client secret for $client_id" >&2
        return 1
    fi

    # 32 random bytes base64-encoded for oauth2-proxy cookie_secret.
    local cookie
    cookie="$(openssl rand -base64 32 | tr -d '\n')"

    jq -n \
        --arg service "$service" \
        --arg client_id "$client_id" \
        --arg secret "$secret" \
        --arg cookie "$cookie" \
        '{service:$service, clientId:$client_id, clientSecret:$secret, cookieSecret:$cookie}'
}

ENTRIES=()
for svc in prometheus alertmanager loki; do
    entry="$(create_or_update_client "$svc" "${PORTS[$svc]}")"
    ENTRIES+=("$entry")
done

# Combine all entries into a single JSON document.
printf '%s\n' "${ENTRIES[@]}" | jq -s '{clients: .}'
