#!/bin/bash
set -euo pipefail

# OpenBao first-boot initialization script.
# Runs once per host to:
# 1. Initialize OpenBao with Shamir key splitting (5 shares, 3 threshold)
# 2. Seal/encrypt unseal keys via TPM Sealer
# 3. Store encrypted blob at /etc/openbao/unseal/keys.enc
# 4. Write root token to /run/openbao/token (readable by openbao user)
# 5. Create initial policies and service accounts
#
# Usage:
#   sudo ./setup-openbao.sh
#
# Prerequisites:
# - openbao binary installed at /usr/bin/bao
# - openbao.service and nova-bao-unseal.service enabled but not started
# - TPM 2.0 device accessible at /dev/tpmrm0 or /dev/tpm0
# - nova-bao-unseal binary at /usr/local/bin/nova-bao-unseal

set -x

BAO_ADDR="${BAO_ADDR:-https://localhost:8200}"
UNSEAL_DIR="/etc/openbao/unseal"
TOKEN_FILE="/run/openbao/token"
OPENBAO_USER="openbao"
OPENBAO_GROUP="openbao"
NOVA_BAO_UNSEAL_BIN="/usr/local/bin/nova-bao-unseal"

# Ensure unseal directory exists and is owned by openbao
mkdir -p "$UNSEAL_DIR"
chown "$OPENBAO_USER:$OPENBAO_GROUP" "$UNSEAL_DIR"
chmod 0700 "$UNSEAL_DIR"

# Ensure /run/openbao exists for token storage
mkdir -p /run/openbao
chown "$OPENBAO_USER:$OPENBAO_GROUP" /run/openbao
chmod 0700 /run/openbao

# Start openbao in the background if not already running
if ! systemctl is-active --quiet openbao.service; then
    echo "Starting openbao.service..."
    systemctl start openbao.service
    sleep 3  # Give it time to start listening
fi

# Wait for openbao to be ready (up to 30s)
echo "Waiting for OpenBao to be ready..."
for i in {1..30}; do
    if curl -sk "$BAO_ADDR/v1/sys/health" &>/dev/null; then
        echo "OpenBao is ready"
        break
    fi
    echo "Attempt $i/30, waiting..."
    sleep 1
done

# Check if already initialized
HEALTH=$(curl -sk -w "%{http_code}" -o /tmp/health.json "$BAO_ADDR/v1/sys/health")
if grep -q '"initialized":true' /tmp/health.json; then
    echo "OpenBao already initialized, skipping init..."
    exit 0
fi

echo "Initializing OpenBao with Shamir split (5 shares, 3 threshold)..."
INIT_RESPONSE=$(mktemp)
curl -sk -X POST "$BAO_ADDR/v1/sys/init" \
    -d '{
        "secret_shares": 5,
        "secret_threshold": 3
    }' | tee "$INIT_RESPONSE"

# Extract root token and unseal keys
ROOT_TOKEN=$(jq -r '.root_token' "$INIT_RESPONSE")
UNSEAL_KEYS=$(jq -r '.keys[]' "$INIT_RESPONSE")

if [[ -z "$ROOT_TOKEN" ]]; then
    echo "Error: failed to extract root token from init response" >&2
    exit 1
fi

echo "Root token: $ROOT_TOKEN"
echo "Unseal keys will be encrypted via TPM and stored at $UNSEAL_DIR/keys.enc"

# Write root token to file (will be used by subsequent init scripts)
echo "$ROOT_TOKEN" > "$TOKEN_FILE"
chown "$OPENBAO_USER:$OPENBAO_GROUP" "$TOKEN_FILE"
chmod 0600 "$TOKEN_FILE"

# Prepare unseal keys file for encryption
UNSEAL_JSON=$(echo "$UNSEAL_KEYS" | jq -R . | jq -s '.')
echo "$UNSEAL_JSON" > /tmp/unseal_keys.json

# Encrypt unseal keys via nova-bao-unseal with TPM
# (nova-bao-unseal --init mode encrypts plaintext and writes to keys.enc)
echo "Encrypting unseal keys via TPM..."
if [[ ! -x "$NOVA_BAO_UNSEAL_BIN" ]]; then
    echo "Error: $NOVA_BAO_UNSEAL_BIN not found or not executable" >&2
    exit 1
fi

"$NOVA_BAO_UNSEAL_BIN" --init < /tmp/unseal_keys.json || {
    echo "Error: failed to encrypt unseal keys" >&2
    exit 1
}

# Verify encrypted file exists
if [[ ! -f "$UNSEAL_DIR/keys.enc" ]]; then
    echo "Error: encrypted keys file not created at $UNSEAL_DIR/keys.enc" >&2
    exit 1
fi

chown "$OPENBAO_USER:$OPENBAO_GROUP" "$UNSEAL_DIR/keys.enc"
chmod 0600 "$UNSEAL_DIR/keys.enc"

echo "Unseal keys encrypted and stored at $UNSEAL_DIR/keys.enc"
rm /tmp/unseal_keys.json "$INIT_RESPONSE"

# Now login and create initial policies
echo "Creating OpenBao policies and service accounts..."
curl -sk -X POST "$BAO_ADDR/v1/auth/token/lookup-self" \
    -H "X-Vault-Token: $ROOT_TOKEN" | jq . || {
    echo "Error: root token validation failed" >&2
    exit 1
}

# Create keycloak policy
echo "Creating keycloak policy..."
curl -sk -X PUT "$BAO_ADDR/v1/sys/policies/acl/keycloak" \
    -H "X-Vault-Token: $ROOT_TOKEN" \
    -d "{\"policy\": $(jq -Rs . < /etc/openbao/keycloak-policy.hcl)}" || {
    echo "Warning: failed to create keycloak policy (may already exist)" >&2
}

# Create novanas policy
echo "Creating novanas policy..."
curl -sk -X PUT "$BAO_ADDR/v1/sys/policies/acl/novanas" \
    -H "X-Vault-Token: $ROOT_TOKEN" \
    -d "{\"policy\": $(jq -Rs . < /etc/openbao/novanas-policy.hcl)}" || {
    echo "Warning: failed to create novanas policy (may already exist)" >&2
}

# Enable KV secrets engine at secret/ mount
echo "Enabling KV v2 secrets engine..."
curl -sk -X POST "$BAO_ADDR/v1/sys/mounts/secret" \
    -H "X-Vault-Token: $ROOT_TOKEN" \
    -d '{"type": "kv", "options": {"version": "2"}}' 2>/dev/null || {
    echo "Warning: KV mount may already exist" >&2
}

# Create service account tokens for keycloak and novanas
echo "Creating service account tokens..."

# Create keycloak token
KC_TOKEN=$(curl -sk -X POST "$BAO_ADDR/v1/auth/token/create" \
    -H "X-Vault-Token: $ROOT_TOKEN" \
    -d '{
        "policies": ["keycloak"],
        "ttl": "8760h",
        "renewable": true
    }' | jq -r '.auth.client_token')

if [[ -z "$KC_TOKEN" || "$KC_TOKEN" == "null" ]]; then
    echo "Error: failed to create keycloak token" >&2
    exit 1
fi

mkdir -p /etc/keycloak
echo "$KC_TOKEN" > /etc/keycloak/bao-token
chown keycloak:keycloak /etc/keycloak/bao-token
chmod 0600 /etc/keycloak/bao-token

# Create novanas token
NOVA_TOKEN=$(curl -sk -X POST "$BAO_ADDR/v1/auth/token/create" \
    -H "X-Vault-Token: $ROOT_TOKEN" \
    -d '{
        "policies": ["novanas"],
        "ttl": "8760h",
        "renewable": true
    }' | jq -r '.auth.client_token')

if [[ -z "$NOVA_TOKEN" || "$NOVA_TOKEN" == "null" ]]; then
    echo "Error: failed to create novanas token" >&2
    exit 1
fi

mkdir -p /etc/nova-api
echo "$NOVA_TOKEN" > /etc/nova-api/bao-token
chown root:root /etc/nova-api/bao-token
chmod 0600 /etc/nova-api/bao-token

echo "OpenBao initialization complete"
echo ""
echo "Summary:"
echo "  Root token: $ROOT_TOKEN (saved to $TOKEN_FILE for future scripts)"
echo "  Unseal keys: encrypted at $UNSEAL_DIR/keys.enc"
echo "  Keycloak token: /etc/keycloak/bao-token"
echo "  NovaNAS token: /etc/nova-api/bao-token"
echo ""
echo "Next steps:"
echo "  1. Stop openbao: systemctl stop openbao.service"
echo "  2. Verify nova-bao-tpm-unseal is enabled: systemctl enable nova-bao-tpm-unseal.service"
echo "  3. Reboot and verify unseal via: systemctl status nova-bao-tpm-unseal.service"
echo "  4. Check OpenBao is unsealed: curl -sk https://localhost:8200/v1/sys/health | jq"
