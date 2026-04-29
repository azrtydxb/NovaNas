#!/usr/bin/env bash
# Create (or refresh) the `rustfs` Keycloak client used by RustFS for OIDC
# IAM federation. Idempotent: re-running updates the client in place and
# rotates the client secret.
#
# Output (JSON on stdout):
#   {"clientId":"rustfs","clientSecret":"..."}
#
# Operator usage:
#
#   ./create-rustfs-client.sh --kc-url https://192.168.10.204:8443 \
#       --admin-pass "$KC_ADMIN_PASS" > /tmp/rustfs.json
#   SECRET=$(jq -r .clientSecret /tmp/rustfs.json)
#   sudo sed -i "s|^RUSTFS_IDENTITY_OPENID_CLIENT_SECRET=.*|RUSTFS_IDENTITY_OPENID_CLIENT_SECRET=$SECRET|" \
#       /etc/rustfs/rustfs.env
#   sudo systemctl restart rustfs.service
#
# This script:
#   - creates a confidential client `rustfs` with both auth-code flow
#     (for the RustFS web console) AND service-account flow (so nova-api
#     or other backend services can mint tokens via client_credentials)
#   - registers the console redirect URI (default: https://novanas.local:9001/oauth_callback)
#   - adds an audience mapper so `rustfs` is in the access token `aud`
#   - adds a "groups" protocol mapper that emits realm_access.roles values
#     into a flat `groups` claim, which is what RustFS reads
#     (RUSTFS_IDENTITY_OPENID_GROUPS_CLAIM=groups)
#   - additionally maps each realm role to a client role of the same name
#     for documentation / kcadm-readability purposes
#
# RustFS itself maps the values it reads from the `groups` claim onto its
# IAM policies. The operator runs the policy-attach step out-of-band — see
# docs/objects/README.md "OIDC policy mapping" for the runbook.
#
# Required env (or pass --kc-* flags):
#   KC_URL        Keycloak base URL (e.g. https://192.168.10.204:8443)
#   KC_REALM      Target realm (default: novanas)
#   KC_ADMIN_USER Keycloak admin user (default: admin)
#   KC_ADMIN_PASS Keycloak admin password
#   KCADM         Path to kcadm.sh (default: kcadm.sh on PATH)
#   REDIRECT_URI  RustFS console OIDC redirect URI
#                 (default: https://novanas.local:9001/oauth_callback)
#   ROOT_URL      RustFS console root URL (default: https://novanas.local:9001)

set -euo pipefail

KC_URL="${KC_URL:-}"
KC_REALM="${KC_REALM:-novanas}"
KC_ADMIN_USER="${KC_ADMIN_USER:-admin}"
KC_ADMIN_PASS="${KC_ADMIN_PASS:-}"
KCADM="${KCADM:-kcadm.sh}"
CLIENT_ID="${CLIENT_ID:-rustfs}"
REDIRECT_URI="${REDIRECT_URI:-https://novanas.local:9001/oauth_callback}"
ROOT_URL="${ROOT_URL:-https://novanas.local:9001}"
TLS_INSECURE="${TLS_INSECURE:-false}"

usage() {
    cat <<EOF >&2
usage: $0 [--kc-url URL] [--realm REALM] [--admin-user USER] [--admin-pass PASS]
          [--client-id ID] [--redirect-uri URI] [--root-url URL] [--insecure]

Creates/updates the '$CLIENT_ID' confidential client in Keycloak (auth-code +
service-account) and prints {"clientId":"...","clientSecret":"..."} to stdout.
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
    : # informational; kcadm honours JAVA_OPTS for truststore
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
  "name": "NovaNAS RustFS object storage",
  "description": "OIDC client for RustFS console (auth-code) and service-to-service (client_credentials)",
  "enabled": true,
  "protocol": "openid-connect",
  "publicClient": false,
  "clientAuthenticatorType": "client-secret",
  "standardFlowEnabled": true,
  "directAccessGrantsEnabled": false,
  "implicitFlowEnabled": false,
  "serviceAccountsEnabled": true,
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

# ---------------------------------------------------------------------------
# Audience mapper: ensure 'rustfs' is in token aud claim.
# ---------------------------------------------------------------------------
AUD_MAPPER_PAYLOAD=$(cat <<JSON
{
  "name": "rustfs-audience",
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

EXISTING_AUD_ID="$($KC get "clients/$EXISTING_ID/protocol-mappers/models" -r "$KC_REALM" --fields id,name --format csv --noquotes 2>/dev/null | awk -F, '$2=="rustfs-audience"{print $1}' | head -n1 || true)"
if [[ -z "$EXISTING_AUD_ID" ]]; then
    $KC create "clients/$EXISTING_ID/protocol-mappers/models" -r "$KC_REALM" -f - <<<"$AUD_MAPPER_PAYLOAD" >/dev/null
fi

# ---------------------------------------------------------------------------
# Groups mapper: emit realm_access.roles values as a flat `groups` claim.
# RustFS reads the `groups` claim (RUSTFS_IDENTITY_OPENID_GROUPS_CLAIM=groups)
# and matches each value against IAM policy mappings.
# ---------------------------------------------------------------------------
GROUPS_MAPPER_PAYLOAD=$(cat <<'JSON'
{
  "name": "rustfs-realm-roles-as-groups",
  "protocol": "openid-connect",
  "protocolMapper": "oidc-usermodel-realm-role-mapper",
  "config": {
    "multivalued": "true",
    "userinfo.token.claim": "true",
    "id.token.claim": "true",
    "access.token.claim": "true",
    "claim.name": "groups",
    "jsonType.label": "String"
  }
}
JSON
)

EXISTING_GROUPS_ID="$($KC get "clients/$EXISTING_ID/protocol-mappers/models" -r "$KC_REALM" --fields id,name --format csv --noquotes 2>/dev/null | awk -F, '$2=="rustfs-realm-roles-as-groups"{print $1}' | head -n1 || true)"
if [[ -z "$EXISTING_GROUPS_ID" ]]; then
    $KC create "clients/$EXISTING_ID/protocol-mappers/models" -r "$KC_REALM" -f - <<<"$GROUPS_MAPPER_PAYLOAD" >/dev/null
fi

# ---------------------------------------------------------------------------
# Optional: explicit client roles mirroring realm roles, for kcadm
# inspectability and to document the rustfs-policy mapping.
# ---------------------------------------------------------------------------
for role in nova-admin nova-operator nova-viewer; do
    $KC create "clients/$EXISTING_ID/roles" -r "$KC_REALM" \
        -s "name=$role" -s "description=Mirror of realm role $role for RustFS policy mapping" >/dev/null 2>&1 || true
done

map_realm_to_client_role() {
    local realm_role="$1"
    local client_role="$2"
    local client_role_obj
    client_role_obj="$($KC get "clients/$EXISTING_ID/roles/$client_role" -r "$KC_REALM" 2>/dev/null || true)"
    if [[ -z "$client_role_obj" ]]; then
        echo "WARN: client role $client_role not found, skipping mapping" >&2
        return 0
    fi
    $KC add-roles -r "$KC_REALM" --rname "$realm_role" \
        --cclientid "$CLIENT_ID" --rolename "$client_role" >/dev/null 2>&1 || true
}

map_realm_to_client_role "nova-admin"    "nova-admin"
map_realm_to_client_role "nova-operator" "nova-operator"
map_realm_to_client_role "nova-viewer"   "nova-viewer"

# ---------------------------------------------------------------------------
# Service account: grant nova-operator so the SA can talk to RustFS for
# bucket lifecycle operations from nova-api (future).
# ---------------------------------------------------------------------------
SVC_USER_ID="$($KC get "clients/$EXISTING_ID/service-account-user" -r "$KC_REALM" --fields id --format csv --noquotes 2>/dev/null | head -n1 || true)"
if [[ -n "$SVC_USER_ID" ]]; then
    $KC add-roles -r "$KC_REALM" --uusername "service-account-${CLIENT_ID}" --rolename nova-operator >/dev/null 2>&1 || \
        $KC add-roles -r "$KC_REALM" --uid "$SVC_USER_ID" --rolename nova-operator >/dev/null 2>&1 || \
        echo "WARN: failed to attach nova-operator to service account" >&2
fi

# ---------------------------------------------------------------------------
# Rotate client secret.
# ---------------------------------------------------------------------------
$KC create "clients/$EXISTING_ID/client-secret" -r "$KC_REALM" >/dev/null 2>&1 || true
SECRET="$($KC get "clients/$EXISTING_ID/client-secret" -r "$KC_REALM" --fields value --format csv --noquotes 2>/dev/null | head -n1)"
if [[ -z "$SECRET" || "$SECRET" == "null" ]]; then
    echo "ERROR: failed to read client secret for $CLIENT_ID" >&2
    exit 1
fi

jq -n --arg id "$CLIENT_ID" --arg secret "$SECRET" \
    '{clientId:$id, clientSecret:$secret}'
