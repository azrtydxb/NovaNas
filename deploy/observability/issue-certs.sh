#!/bin/bash
# issue-certs.sh: Issue self-signed TLS certs for observability stack from local CA
# Usage: bash issue-certs.sh [--force]
# Assumes /etc/nova-ca/ca.crt and /etc/nova-ca/ca.key exist

set -euo pipefail

CA_CRT="/etc/nova-ca/ca.crt"
CA_KEY="/etc/nova-ca/ca.key"
CERT_DIR="/etc/nova-certs"
DAYS_VALID=365

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $*"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARN:${NC} $*"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR:${NC} $*" >&2
}

# Verify CA exists
if [[ ! -f "$CA_CRT" ]]; then
    error "CA cert not found at $CA_CRT. Please set up /etc/nova-ca first."
    exit 1
fi

if [[ ! -f "$CA_KEY" ]]; then
    error "CA key not found at $CA_KEY"
    exit 1
fi

# Create cert directory
mkdir -p "$CERT_DIR"
sudo chmod 0755 "$CERT_DIR"

# Parse --force flag
FORCE=0
if [[ "${1:-}" == "--force" ]]; then
    FORCE=1
fi

issue_cert() {
    local name=$1
    local cn=$2
    local san=$3

    local crt="$CERT_DIR/${name}.crt"
    local key="$CERT_DIR/${name}.key"
    local csr="$CERT_DIR/${name}.csr"

    # Skip if cert exists and is valid
    if [[ -f "$crt" && $FORCE -eq 0 ]]; then
        if openssl x509 -in "$crt" -noout -checkend 86400 >/dev/null 2>&1; then
            log "✓ Cert $name already valid, skipping"
            return 0
        fi
    fi

    log "Issuing cert for $name (CN=$cn)"

    # Generate CSR
    openssl req -new -newkey rsa:2048 -nodes \
        -keyout "$key" \
        -out "$csr" \
        -subj "/CN=$cn" \
        2>/dev/null

    # Sign CSR with CA
    openssl x509 -req \
        -in "$csr" \
        -CA "$CA_CRT" \
        -CAkey "$CA_KEY" \
        -CAcreateserial \
        -out "$crt" \
        -days "$DAYS_VALID" \
        -extensions v3_req \
        -extfile <(echo "subjectAltName=$san") \
        2>/dev/null

    # Clean up CSR
    rm -f "$csr"

    # Fix permissions
    sudo chmod 0644 "$crt"
    sudo chmod 0600 "$key"

    log "✓ Issued $name cert"
}

# Issue certs for each component
issue_cert "prometheus" "prometheus.novanas.local" "DNS:127.0.0.1,DNS:prometheus.novanas.local,IP:127.0.0.1"
issue_cert "alertmanager" "alertmanager.novanas.local" "DNS:127.0.0.1,DNS:alertmanager.novanas.local,IP:127.0.0.1"
issue_cert "grafana" "grafana.novanas.local" "DNS:127.0.0.1,DNS:grafana.novanas.local,IP:127.0.0.1"
issue_cert "loki" "loki.novanas.local" "DNS:127.0.0.1,DNS:loki.novanas.local,IP:127.0.0.1"

log "All certs issued successfully"
