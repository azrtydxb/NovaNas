#!/usr/bin/env bash
# Create (or refresh) the `nova-csi` Keycloak client for the OAuth2
# client_credentials grant used by the NovaNAS CSI driver. Idempotent:
# safe to re-run — the client is updated in place, and a fresh client
# secret is generated and printed to stdout as JSON:
#
#   {"clientId":"nova-csi","clientSecret":"..."}
#
# Operator usage:
#
#   ./create-csi-client.sh > /tmp/csi.json
#   SECRET=$(jq -r .clientSecret /tmp/csi.json)
#   kubectl -n nova-csi create secret generic nova-csi-auth \
#     --from-literal=oidc-client-id=nova-csi \
#     --from-literal=oidc-client-secret="$SECRET" \
#     --from-file=ca.crt=/etc/nova-ca/ca.crt
#
# Required env (or pass --kc-* flags below):
#   KC_URL        Keycloak base URL (e.g. https://kc:8443)
#   KC_REALM      Target realm (default: novanas)
#   KC_ADMIN_USER Keycloak admin user (default: admin)
#   KC_ADMIN_PASS Keycloak admin password
#   KCADM         Path to kcadm.sh (default: kcadm.sh on PATH)
#
# Notes:
#   - Requires the realm to already exist with the `nova-operator` realm
#     role (this is true for the standard nova realm import).
#   - Uses --no-config so it does not pollute ~/.keycloak.

set -euo pipefail

KC_URL="${KC_URL:-}"
KC_REALM="${KC_REALM:-novanas}"
KC_ADMIN_USER="${KC_ADMIN_USER:-admin}"
KC_ADMIN_PASS="${KC_ADMIN_PASS:-}"
KCADM="${KCADM:-kcadm.sh}"
CLIENT_ID="${CLIENT_ID:-nova-csi}"
TLS_INSECURE="${TLS_INSECURE:-false}"

usage() {
    cat <<EOF >&2
usage: $0 [--kc-url URL] [--realm REALM] [--admin-user USER] [--admin-pass PASS] [--client-id ID] [--insecure]

Creates/updates the '$CLIENT_ID' confidential service-account client in
Keycloak and prints {"clientId":"...","clientSecret":"..."} to stdout.
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

# Configure kcadm with explicit truststore handling.
TLS_FLAG=()
if [[ "$TLS_INSECURE" == "true" ]]; then
    TLS_FLAG=(--truststore "" --trustpass "")
fi

$KC config credentials \
    --server "$KC_URL" \
    --realm master \
    --user "$KC_ADMIN_USER" \
    --password "$KC_ADMIN_PASS" >/dev/null

# Create or update the client. kcadm get clients filters with -q.
EXISTING_ID="$($KC get clients -r "$KC_REALM" -q "clientId=$CLIENT_ID" --fields id --format csv --noquotes 2>/dev/null | head -n1 || true)"

CLIENT_PAYLOAD=$(cat <<JSON
{
  "clientId": "$CLIENT_ID",
  "name": "NovaNAS CSI driver",
  "description": "Service account for the NovaNAS CSI driver (client_credentials)",
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

# Map the realm role nova-operator to the service account user.
SVC_USER_ID="$($KC get "clients/$EXISTING_ID/service-account-user" -r "$KC_REALM" --fields id --format csv --noquotes 2>/dev/null | head -n1)"
if [[ -z "$SVC_USER_ID" ]]; then
    echo "ERROR: could not resolve service-account user for client $CLIENT_ID" >&2
    exit 1
fi

# add-roles is idempotent — re-adding an existing role is a no-op.
$KC add-roles -r "$KC_REALM" --uusername "service-account-${CLIENT_ID}" --rolename nova-operator >/dev/null 2>&1 || \
    $KC add-roles -r "$KC_REALM" --uid "$SVC_USER_ID" --rolename nova-operator >/dev/null

# Rotate the client secret to guarantee the operator gets a known value.
NEW_SECRET="$($KC create "clients/$EXISTING_ID/client-secret" -r "$KC_REALM" -i 2>/dev/null || true)"
# Some kcadm versions return the secret as JSON in stdout; otherwise GET it.
SECRET="$($KC get "clients/$EXISTING_ID/client-secret" -r "$KC_REALM" --fields value --format csv --noquotes 2>/dev/null | head -n1)"
if [[ -z "$SECRET" || "$SECRET" == "null" ]]; then
    echo "ERROR: failed to read client secret for $CLIENT_ID" >&2
    exit 1
fi

jq -n --arg id "$CLIENT_ID" --arg secret "$SECRET" '{clientId:$id, clientSecret:$secret}'
