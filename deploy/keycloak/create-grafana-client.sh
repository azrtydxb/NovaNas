#!/usr/bin/env bash
# Create (or refresh) the `grafana` Keycloak client used by Grafana's
# native OIDC (auth.generic_oauth) integration. Idempotent: re-running
# the script updates the client in place and rotates the client secret.
#
# Output (JSON on stdout):
#   {"clientId":"grafana","clientSecret":"..."}
#
# Operator usage:
#
#   ./create-grafana-client.sh > /tmp/grafana.json
#   SECRET=$(jq -r .clientSecret /tmp/grafana.json)
#   echo -n "$SECRET" | sudo install -m 0400 -o grafana -g grafana \
#     /dev/stdin /etc/grafana/oidc-secret
#   sudo systemctl restart grafana.service
#
# This script:
#   - creates a confidential auth-code-flow client `grafana`
#   - registers redirect URI for /login/generic_oauth
#   - adds an audience mapper so `grafana` appears in token `aud`
#   - adds a realm-roles client-scope mapping so realm_access.roles is
#     populated (Grafana role_attribute_path needs this)
#   - maps realm roles nova-admin/nova-operator/nova-viewer to client
#     roles Admin/Editor/Viewer (informational; Grafana actually uses
#     role_attribute_path JMESPath against realm_access.roles)
#
# Required env (or pass --kc-* flags):
#   KC_URL        Keycloak base URL (e.g. https://192.168.10.204:8443)
#   KC_REALM      Target realm (default: novanas)
#   KC_ADMIN_USER Keycloak admin user (default: admin)
#   KC_ADMIN_PASS Keycloak admin password
#   KCADM         Path to kcadm.sh (default: kcadm.sh on PATH)
#   REDIRECT_URI  Grafana redirect URI
#                 (default: https://novanas.local:3000/login/generic_oauth)
#   ROOT_URL      Grafana root URL
#                 (default: https://novanas.local:3000)

set -euo pipefail

KC_URL="${KC_URL:-}"
KC_REALM="${KC_REALM:-novanas}"
KC_ADMIN_USER="${KC_ADMIN_USER:-admin}"
KC_ADMIN_PASS="${KC_ADMIN_PASS:-}"
KCADM="${KCADM:-kcadm.sh}"
CLIENT_ID="${CLIENT_ID:-grafana}"
REDIRECT_URI="${REDIRECT_URI:-https://novanas.local:3000/login/generic_oauth}"
ROOT_URL="${ROOT_URL:-https://novanas.local:3000}"
TLS_INSECURE="${TLS_INSECURE:-false}"

usage() {
    cat <<EOF >&2
usage: $0 [--kc-url URL] [--realm REALM] [--admin-user USER] [--admin-pass PASS]
          [--client-id ID] [--redirect-uri URI] [--root-url URL] [--insecure]

Creates/updates the '$CLIENT_ID' confidential auth-code client in Keycloak
for Grafana OIDC and prints {"clientId":"...","clientSecret":"..."} to stdout.
EOF
    exit 2
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --kc-url) KC_URL="$2"; shift 2 ;;
        --realm) KC_REALM="$2"; shift 2 ;;
        --admin-user) KC_ADMIN_USER="$2"; shift 2 ;;
        --admin-pass) KC_ADMIN_PASS="$2"; shift 2 ;;
        --client-id) CLIENT_ID="$2"; shift 2 ;;
        --redirect-uri) REDIRECT_URI="$2"; shift 2 ;;
        --root-url) ROOT_URL="$2"; shift 2 ;;
        --insecure) TLS_INSECURE="true"; shift ;;
        -h|--help) usage ;;
        *) echo "unknown arg: $1" >&2; usage ;;
    esac
done

[[ -n "$KC_URL" ]] || { echo "ERROR: KC_URL or --kc-url required" >&2; exit 2; }
[[ -n "$KC_ADMIN_PASS" ]] || { echo "ERROR: KC_ADMIN_PASS or --admin-pass required" >&2; exit 2; }
command -v "$KCADM" >/dev/null 2>&1 || { echo "ERROR: kcadm.sh ($KCADM) not found in PATH" >&2; exit 2; }
command -v jq >/dev/null 2>&1 || { echo "ERROR: jq required" >&2; exit 2; }

CFGDIR="$(mktemp -d)"
trap 'rm -rf "$CFGDIR"' EXIT
# kcadm.sh on recent Keycloak only accepts --config AFTER the subcommand,
# so we isolate per-invocation state via HOME instead. CFGDIR is a tmpdir
# the trap above cleans up.
export HOME="$CFGDIR"
KC="$KCADM"

if [[ "$TLS_INSECURE" == "true" ]]; then
    : # informational, kcadm honours JAVA_OPTS for truststore
fi

$KC config credentials \
    --server "$KC_URL" \
    --realm master \
    --user "$KC_ADMIN_USER" \
    --password "$KC_ADMIN_PASS" >/dev/null

EXISTING_ID="$($KC get clients -r "$KC_REALM" -q "clientId=$CLIENT_ID" --fields id --format csv --noquotes 2>/dev/null | head -n1 || true)"

CLIENT_PAYLOAD=$(cat <<JSON
{
  "clientId": "$CLIENT_ID",
  "name": "NovaNAS Grafana",
  "description": "Grafana auth-code OIDC client for NovaNAS observability",
  "enabled": true,
  "protocol": "openid-connect",
  "publicClient": false,
  "clientAuthenticatorType": "client-secret",
  "standardFlowEnabled": true,
  "directAccessGrantsEnabled": false,
  "implicitFlowEnabled": false,
  "serviceAccountsEnabled": false,
  "frontchannelLogout": true,
  "rootUrl": "$ROOT_URL",
  "baseUrl": "$ROOT_URL",
  "redirectUris": ["$REDIRECT_URI"],
  "webOrigins": ["$ROOT_URL"],
  "attributes": {
    "post.logout.redirect.uris": "$ROOT_URL/*",
    "pkce.code.challenge.method": "S256"
  },
  "defaultClientScopes": ["web-origins", "profile", "roles", "email"],
  "optionalClientScopes": ["address", "phone", "offline_access", "microprofile-jwt"]
}
JSON
)

if [[ -z "$EXISTING_ID" ]]; then
    echo "Creating client '$CLIENT_ID' in realm '$KC_REALM'" >&2
    $KC create clients -r "$KC_REALM" -f - <<<"$CLIENT_PAYLOAD" >/dev/null && \
        EXISTING_ID="$($KC get clients -r "$KC_REALM" -q "clientId=$CLIENT_ID" --fields id --format csv --noquotes 2>/dev/null | head -n1)"
else
    echo "Updating existing client '$CLIENT_ID' (id=$EXISTING_ID)" >&2
    $KC update "clients/$EXISTING_ID" -r "$KC_REALM" -f - <<<"$CLIENT_PAYLOAD"
fi

# Audience mapper: ensure 'grafana' is in token aud claim.
AUD_MAPPER_PAYLOAD=$(cat <<JSON
{
  "name": "grafana-audience",
  "protocol": "openid-connect",
  "protocolMapper": "oidc-audience-mapper",
  "config": {
    "included.client.audience": "$CLIENT_ID",
    "id.token.claim": "true",
    "access.token.claim": "true"
  }
}
JSON
)

EXISTING_MAPPER_ID="$($KC get "clients/$EXISTING_ID/protocol-mappers/models" -r "$KC_REALM" --fields id,name --format csv --noquotes 2>/dev/null | awk -F, '$2=="grafana-audience"{print $1}' | head -n1 || true)"
if [[ -z "$EXISTING_MAPPER_ID" ]]; then
    $KC create "clients/$EXISTING_ID/protocol-mappers/models" -r "$KC_REALM" -f - <<<"$AUD_MAPPER_PAYLOAD" >/dev/null
fi

# Create informational client roles Admin/Editor/Viewer (Grafana itself
# uses role_attribute_path JMESPath against realm_access.roles, but the
# spec calls for explicit client-role mapping for completeness).
for role in Admin Editor Viewer; do
    $KC create "clients/$EXISTING_ID/roles" -r "$KC_REALM" \
        -s "name=$role" -s "description=Grafana $role role" >/dev/null 2>&1 || true
done

# Map realm roles -> grafana client roles via composite roles.
map_realm_to_client_role() {
    local realm_role="$1"
    local client_role="$2"
    local client_role_obj
    client_role_obj="$($KC get "clients/$EXISTING_ID/roles/$client_role" -r "$KC_REALM" 2>/dev/null || true)"
    if [[ -z "$client_role_obj" ]]; then
        echo "WARN: client role $client_role not found, skipping mapping" >&2
        return 0
    fi
    # Add the grafana client role as a composite under the realm role.
    $KC add-roles -r "$KC_REALM" --rname "$realm_role" \
        --cclientid "$CLIENT_ID" --rolename "$client_role" >/dev/null 2>&1 || true
}

map_realm_to_client_role "nova-admin" "Admin"
map_realm_to_client_role "nova-operator" "Editor"
map_realm_to_client_role "nova-viewer" "Viewer"

# Rotate client secret.
$KC create "clients/$EXISTING_ID/client-secret" -r "$KC_REALM" >/dev/null 2>&1 || true
SECRET="$($KC get "clients/$EXISTING_ID/client-secret" -r "$KC_REALM" --fields value --format csv --noquotes 2>/dev/null | head -n1)"
if [[ -z "$SECRET" || "$SECRET" == "null" ]]; then
    echo "ERROR: failed to read client secret for $CLIENT_ID" >&2
    exit 1
fi

jq -n --arg id "$CLIENT_ID" --arg secret "$SECRET" \
    '{clientId:$id, clientSecret:$secret}'
