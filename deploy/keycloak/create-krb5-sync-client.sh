#!/usr/bin/env bash
# Create (or refresh) the `nova-krb5-sync` Keycloak client used by the
# Keycloak ↔ KDC user-principal sync daemon. The client is a
# confidential service-account client (client_credentials grant). It is
# granted:
#
#   - The realm role `nova-operator` (so the bearer can hit
#     PermKrb5Read+Write endpoints on nova-api).
#   - The realm-management client roles `view-users` and `view-events`
#     (so the daemon can list users and admin events via the Keycloak
#     admin REST API).
#
# Idempotent: re-running rotates the client secret and prints
#   {"clientId":"nova-krb5-sync","clientSecret":"..."} to stdout.
#
# Operator usage:
#
#   ./create-krb5-sync-client.sh > /tmp/sync.json
#   SECRET=$(jq -r .clientSecret /tmp/sync.json)
#   install -m 0640 -o nova-krb5-sync -g nova-krb5-sync \
#     <(printf '%s' "$SECRET") /etc/nova-krb5-sync/oidc-client-secret
#
# Required env (or pass --kc-* flags below):
#   KC_URL        Keycloak base URL (e.g. https://kc:8443)
#   KC_REALM      Target realm (default: novanas)
#   KC_ADMIN_USER Keycloak admin user (default: admin)
#   KC_ADMIN_PASS Keycloak admin password
#   KCADM         Path to kcadm.sh (default: kcadm.sh on PATH)

set -euo pipefail

KC_URL="${KC_URL:-}"
KC_REALM="${KC_REALM:-novanas}"
KC_ADMIN_USER="${KC_ADMIN_USER:-admin}"
KC_ADMIN_PASS="${KC_ADMIN_PASS:-}"
KCADM="${KCADM:-kcadm.sh}"
CLIENT_ID="${CLIENT_ID:-nova-krb5-sync}"
TLS_INSECURE="${TLS_INSECURE:-false}"

usage() {
    cat <<EOF >&2
usage: $0 [--kc-url URL] [--realm REALM] [--admin-user USER] [--admin-pass PASS] [--client-id ID] [--insecure]

Creates/updates the '$CLIENT_ID' confidential service-account client in
Keycloak (with view-users + view-events realm-management roles, plus the
nova-operator realm role) and prints {"clientId":"...","clientSecret":"..."}
to stdout.
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
    : # kcadm respects --truststore via env if needed; left to operator
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
  "name": "NovaNAS krb5 sync daemon",
  "description": "Service account for the Keycloak ↔ KDC user-principal sync (client_credentials).",
  "enabled": true,
  "protocol": "openid-connect",
  "publicClient": false,
  "clientAuthenticatorType": "client-secret",
  "serviceAccountsEnabled": true,
  "standardFlowEnabled": false,
  "directAccessGrantsEnabled": false,
  "implicitFlowEnabled": false,
  "frontchannelLogout": false,
  "attributes": {
    "use.refresh.tokens": "false",
    "client_credentials.use_refresh_token": "false"
  }
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

# Grant realm role nova-operator to the service account.
SVC_USER_ID="$($KC get "clients/$EXISTING_ID/service-account-user" -r "$KC_REALM" --fields id --format csv --noquotes 2>/dev/null | head -n1)"
if [[ -z "$SVC_USER_ID" ]]; then
    echo "ERROR: could not resolve service-account user for client $CLIENT_ID" >&2
    exit 1
fi

$KC add-roles -r "$KC_REALM" --uusername "service-account-${CLIENT_ID}" --rolename nova-operator >/dev/null 2>&1 || \
    $KC add-roles -r "$KC_REALM" --uid "$SVC_USER_ID" --rolename nova-operator >/dev/null

# Grant realm-management client roles: view-users, view-events. These are
# what gates the Keycloak admin REST API endpoints we hit.
RM_CLIENT_ID="$($KC get clients -r "$KC_REALM" -q clientId=realm-management --fields id --format csv --noquotes 2>/dev/null | head -n1)"
if [[ -z "$RM_CLIENT_ID" ]]; then
    echo "ERROR: realm-management client not found in realm $KC_REALM" >&2
    exit 1
fi

for role in view-users view-events; do
    $KC add-roles -r "$KC_REALM" --uusername "service-account-${CLIENT_ID}" \
        --cclientid realm-management --rolename "$role" >/dev/null 2>&1 || \
    $KC add-roles -r "$KC_REALM" --uid "$SVC_USER_ID" \
        --cclientid realm-management --rolename "$role" >/dev/null
done

# Rotate the secret to guarantee the operator gets a known value.
_=$($KC create "clients/$EXISTING_ID/client-secret" -r "$KC_REALM" -i 2>/dev/null || true)
SECRET="$($KC get "clients/$EXISTING_ID/client-secret" -r "$KC_REALM" --fields value --format csv --noquotes 2>/dev/null | head -n1)"
if [[ -z "$SECRET" || "$SECRET" == "null" ]]; then
    echo "ERROR: failed to read client secret for $CLIENT_ID" >&2
    exit 1
fi

jq -n --arg id "$CLIENT_ID" --arg secret "$SECRET" '{clientId:$id, clientSecret:$secret}'
