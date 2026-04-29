#!/bin/bash
set -euo pipefail

# Idempotent script to initialize Keycloak database and user.
# Reads DB password from OpenBao at secret/keycloak/db, creates user+db if missing.
#
# Usage:
#   BAO_ADDR=https://localhost:8200 BAO_TOKEN_FILE=/run/openbao/token ./init-keycloak-db.sh

BAO_ADDR="${BAO_ADDR:-https://localhost:8200}"
BAO_TOKEN_FILE="${BAO_TOKEN_FILE:-/run/openbao/token}"
DB_NAME="${DB_NAME:-keycloak}"
DB_USER="${DB_USER:-keycloak}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"

if [[ ! -f "$BAO_TOKEN_FILE" ]]; then
    echo "Error: token file not found at $BAO_TOKEN_FILE" >&2
    exit 1
fi

BAO_TOKEN=$(cat "$BAO_TOKEN_FILE")

# Fetch password from OpenBao, or generate one
echo "Fetching Keycloak DB password from OpenBao..."
PASSWORD=$(curl -s \
    -H "X-Vault-Token: $BAO_TOKEN" \
    "$BAO_ADDR/v1/secret/data/keycloak/db" \
    | jq -r '.data.data.password' 2>/dev/null || echo "")

if [[ -z "$PASSWORD" ]]; then
    echo "Password not found in OpenBao, generating and storing new one..."
    PASSWORD=$(openssl rand -base64 32)

    # Store in OpenBao
    curl -s -X POST \
        -H "X-Vault-Token: $BAO_TOKEN" \
        -d "{\"data\": {\"password\": \"$PASSWORD\"}}" \
        "$BAO_ADDR/v1/secret/data/keycloak/db" > /dev/null

    echo "Password stored in OpenBao at secret/keycloak/db"
fi

# Create database and user if they don't exist
echo "Creating Keycloak database and user..."
sudo -u "$POSTGRES_USER" psql << EOF
SELECT 1 FROM pg_database WHERE datname = '$DB_NAME' \G
\if :?
    -- Database exists, skip creation
    \else
    CREATE DATABASE "$DB_NAME" ENCODING 'UTF8' LC_COLLATE 'en_US.UTF-8' LC_CTYPE 'en_US.UTF-8';
    GRANT ALL PRIVILEGES ON DATABASE "$DB_NAME" TO "$DB_USER";
\endif

SELECT 1 FROM pg_roles WHERE rolname = '$DB_USER' \G
\if :?
    -- User exists, update password
    ALTER USER "$DB_USER" WITH PASSWORD '$PASSWORD';
    \else
    -- Create new user
    CREATE USER "$DB_USER" WITH PASSWORD '$PASSWORD';
    GRANT ALL PRIVILEGES ON DATABASE "$DB_NAME" TO "$DB_USER";
\endif
EOF

echo "Keycloak database initialized successfully"
