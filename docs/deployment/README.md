# NovaNAS Deployment Guide

This guide walks through deploying NovaNAS on a single-node Debian server with the auth/secrets/TLS infrastructure running on systemd (not k3s).

## Architecture Overview

NovaNAS infrastructure follows a strict boot ordering to handle circular dependencies:

```
postgresql (primary data store)
    ↓
openbao (TPM-backed secret management)
    ↓
nova-bao-tpm-unseal (decrypts unseal keys at boot)
    ↓
keycloak (identity provider, needs openbao + postgres)
    ↓
nova-api (REST API, validates JWTs from keycloak)
    ↓
k3s (user workloads, provided storage via nova-api)
```

### Design Constraints

**All infrastructure services run on systemd, not k3s.** This is intentional:

- k3s storage is provided by NovaNAS via iSCSI/NVMe-oF, creating a circular dependency if NovaNAS runs on k3s
- PostgreSQL, OpenBao, and Keycloak are persistent infrastructure — best run on host systemd
- k3s is reserved exclusively for user workloads (containers, VMs via Kubevirt later)

### Core Services

| Service | Purpose | Port | Persistence | User |
|---------|---------|------|-------------|------|
| `postgresql` | Primary DB for nova-api + keycloak | 5432 | `/var/lib/postgresql` | `postgres` |
| `openbao` | Secret store, encryption, PKI CA | 8200 | `/var/lib/openbao` (raft) | `openbao` |
| `keycloak` | OIDC identity provider | 8080 | Keycloak DB in postgresql | `keycloak` |
| `nova-api` | NovaNAS storage control plane | 8443 | Via postgresql | `root` |
| `k3s` | Kubernetes for user workloads | 6443 | Via nova-api storage | `root` |

## Hardware Requirements

### Minimum

- **CPU**: 4 cores (x86_64)
- **RAM**: 16 GB
- **Storage**: 
  - 100 GB for OS + data (SSD recommended for performance)
  - Additional drives for ZFS pool(s) as needed
- **TPM**: 2.0 device (strongly recommended for secure unseal; optional for dev)
- **Network**: 1 GbE (10 GbE recommended for storage workloads)

### Recommended

- **CPU**: 8+ cores
- **RAM**: 32+ GB
- **Storage**: NVMe SSD for OS, separate fast storage for hot data
- **TPM**: 2.0 fTPM (firmware) or discrete module
- **Network**: 10 GbE or higher

## Step-by-Step Installation

### 1. OS Installation

Install Debian 13 (or Ubuntu 24.04 LTS) on the target host:

```bash
# During installer, use entire disk (or allocate dedicated ZFS partition)
# Enable SSH server
# Add to sudo users if not root
```

After boot, update and install core dependencies:

```bash
sudo apt update
sudo apt upgrade -y
sudo apt install -y \
    curl wget jq vim \
    postgresql postgresql-contrib \
    redis-server \
    build-essential git golang-1.25 \
    tpm2-tools libtpm2-pkcs11-1 \
    libssl-dev libpq-dev \
    ca-certificates
```

### 2. ZFS Setup (if using ZFS for storage)

```bash
sudo apt install -y zfsutils-linux

# Create pool (example with /dev/sdb)
sudo zpool create -f -o ashift=12 tank /dev/sdb

# Enable compression and deduplication as needed
sudo zfs set compression=lz4 tank
```

### 3. PostgreSQL Setup

Initialize PostgreSQL and create the novanas database:

```bash
sudo systemctl start postgresql
sudo systemctl enable postgresql

# Create novanas database and user
sudo -u postgres psql << 'EOF'
CREATE DATABASE novanas;
CREATE USER novanas WITH PASSWORD 'change-me';
ALTER ROLE novanas WITH CREATEDB;
GRANT ALL PRIVILEGES ON DATABASE novanas TO novanas;
\c novanas
GRANT ALL ON SCHEMA public TO novanas;
EOF

# Apply schema (after building nova-api)
make migrate-up DB_URL='postgres://novanas:change-me@localhost:5432/novanas?sslmode=disable'
```

### 4. OpenBao Installation

Download and install OpenBao from openbao.org:

```bash
# Check latest version at https://github.com/openbao/openbao/releases
OPENBAO_VERSION=1.1.0
cd /tmp
wget https://releases.openbao.org/openbao/${OPENBAO_VERSION}/openbao_${OPENBAO_VERSION}_linux_amd64.zip
unzip openbao_*_linux_amd64.zip
sudo mv openbao /usr/bin/bao
sudo chmod 755 /usr/bin/bao
rm openbao_*_linux_amd64.zip

# Verify
bao version
```

Create openbao user and directories:

```bash
sudo useradd --system --home /var/lib/openbao --shell /bin/false openbao

sudo mkdir -p /var/lib/openbao /etc/openbao/tls /etc/openbao/unseal
sudo chown -R openbao:openbao /var/lib/openbao /etc/openbao
sudo chmod 0700 /var/lib/openbao /etc/openbao /etc/openbao/unseal
```

Generate TLS certificates for OpenBao:

```bash
# Self-signed for this example (use proper CA in production)
sudo openssl req -x509 -newkey rsa:4096 -keyout /etc/openbao/tls/key.pem \
    -out /etc/openbao/tls/cert.pem -days 3650 -nodes \
    -subj "/CN=openbao.novanas.local"

sudo chown openbao:openbao /etc/openbao/tls/*.pem
sudo chmod 0600 /etc/openbao/tls/*.pem
```

Copy configuration files:

```bash
sudo cp deploy/openbao/openbao.hcl /etc/openbao/
sudo cp deploy/openbao/*-policy.hcl /etc/openbao/
sudo chown openbao:openbao /etc/openbao/*.hcl
sudo chmod 0644 /etc/openbao/*.hcl
```

### 5. Build Deployment Binaries

Build all binaries including nova-bao-unseal:

```bash
make deploy-bin

# Verify all binaries exist
ls -la bin/
  bin/nova-api
  bin/nova-nvmet-restore
  bin/nova-iscsi-restore
  bin/nova-bao-unseal
```

Install systemd units:

```bash
sudo cp deploy/systemd/*.service /etc/systemd/system/

# Install helper scripts
sudo mkdir -p /usr/local/bin
sudo cp deploy/keycloak/nova-keycloak-bootstrap.sh /usr/local/bin/nova-keycloak-bootstrap
sudo chmod 0755 /usr/local/bin/nova-keycloak-bootstrap
sudo cp bin/nova-bao-unseal /usr/local/bin/
sudo chmod 0755 /usr/local/bin/nova-bao-unseal

# Install NovaNAS binaries
sudo cp bin/nova-api /usr/bin/
sudo cp bin/nova-nvmet-restore /usr/local/bin/
sudo cp bin/nova-iscsi-restore /usr/local/bin/
sudo chmod 0755 /usr/bin/nova-api /usr/local/bin/nova-*

# Reload systemd
sudo systemctl daemon-reload
```

### 6. Initialize OpenBao

Enable units (but don't start yet):

```bash
sudo systemctl enable openbao.service nova-bao-tpm-unseal.service
```

Run the initialization script:

```bash
sudo bash deploy/openbao/setup-openbao.sh
```

This script:
- Starts openbao.service
- Initializes it with Shamir split (5 shares, 3 threshold)
- Encrypts unseal keys via TPM and saves to `/etc/openbao/unseal/keys.enc`
- Creates service account tokens for keycloak and nova-api
- Stops openbao at the end

**Important**: Save the root token from the output. It's needed for manual vault operations.

### 7. Keycloak Setup

Install Java and download Keycloak (Quarkus distribution):

```bash
sudo apt install -y openjdk-21-jre-headless

# Download Keycloak
KEYCLOAK_VERSION=25.0.0
cd /tmp
wget https://github.com/keycloak/keycloak/releases/download/${KEYCLOAK_VERSION}/keycloak-${KEYCLOAK_VERSION}.tar.gz
tar -xzf keycloak-*.tar.gz
sudo mv keycloak-*/ /opt/keycloak
sudo chown -R keycloak:keycloak /opt/keycloak

# Create keycloak user
sudo useradd --system --home /var/lib/keycloak --shell /bin/false keycloak

# Create keycloak config directory
sudo mkdir -p /etc/keycloak
sudo chown keycloak:keycloak /etc/keycloak
sudo chmod 0755 /etc/keycloak
```

Initialize Keycloak database:

```bash
# Ensure postgresql is running
sudo systemctl start postgresql

# Run init script
sudo bash deploy/postgres/init-keycloak-db.sh
```

### 8. Start Services in Order

Boot the infrastructure stack in the correct order:

```bash
# 1. PostgreSQL (already running)
sudo systemctl start postgresql

# 2. OpenBao + TPM unseal
sudo systemctl start openbao.service
sudo systemctl start nova-bao-tpm-unseal.service

# Verify OpenBao is unsealed
sleep 5
curl -sk https://localhost:8200/v1/sys/health | jq .

# 3. Keycloak
sudo systemctl start keycloak.service

# Monitor startup (takes 1-2 min)
sudo journalctl -u keycloak.service -f

# 4. NovaNAS API
sudo systemctl start nova-api.service

# 5. k3s (if deploying)
# sudo systemctl start k3s.service
```

Enable all units to start on boot:

```bash
sudo systemctl enable postgresql
sudo systemctl enable redis-server
sudo systemctl enable openbao.service nova-bao-tpm-unseal.service
sudo systemctl enable keycloak.service
sudo systemctl enable nova-api.service
```

### 9. First-Time Admin Access

#### Initialize Keycloak Realm

Import the pre-built realm configuration:

```bash
# Access Keycloak admin console (port 8080)
# Visit: http://localhost:8080/admin
# Default credentials: admin / CHANGE_ME_ON_FIRST_LOGIN (from realm-novanas.json)

# OR import realm via API:
BAO_TOKEN=$(cat /etc/keycloak/bao-token)
curl -X POST \
    -H "X-Vault-Token: $BAO_TOKEN" \
    https://localhost:8200/v1/secret/data/keycloak/realm-import \
    -d @deploy/keycloak/realm-novanas.json
```

#### Generate Initial Admin JWT

For the first API call to nova-api, use the admin user:

```bash
# Get Keycloak service account credentials
BAO_ADDR=https://localhost:8200
BAO_TOKEN=$(cat /etc/keycloak/bao-token)

# Get token from Keycloak (adjust URL and credentials as needed)
curl -X POST http://localhost:8080/realms/novanas/protocol/openid-connect/token \
    -d "grant_type=password" \
    -d "client_id=nova-api" \
    -d "username=admin" \
    -d "password=CHANGE_ME_ON_FIRST_LOGIN" | jq -r '.access_token'

# Use the token in Authorization header:
# curl -H "Authorization: Bearer <token>" https://localhost:8443/api/v1/...
```

## Verification Checklist

```bash
# PostgreSQL
sudo -u postgres psql -l | grep novanas

# Redis
redis-cli PING

# OpenBao
curl -sk https://localhost:8200/v1/sys/health | jq .

# Keycloak
curl -sk http://localhost:8080/realms/novanas/.well-known/openid-configuration | jq .

# NovaNAS API
curl -sk https://localhost:8443/health

# Check boot order
sudo systemctl status openbao.service nova-bao-tpm-unseal.service keycloak.service nova-api.service
```

## Backup & Recovery

### Backup Procedure

Run daily (cron job recommended):

```bash
#!/bin/bash
BACKUP_DIR=/mnt/backups/novanas
DATE=$(date +%Y%m%d-%H%M%S)

# PostgreSQL dump
sudo -u postgres pg_dump novanas | gzip > $BACKUP_DIR/novanas-db-$DATE.sql.gz
sudo -u postgres pg_dump keycloak | gzip > $BACKUP_DIR/keycloak-db-$DATE.sql.gz

# OpenBao raft snapshot (requires token)
BAO_TOKEN=$(cat /run/openbao/token 2>/dev/null || echo "")
if [[ -n "$BAO_TOKEN" ]]; then
    curl -sk -H "X-Vault-Token: $BAO_TOKEN" \
        https://localhost:8200/v1/sys/storage/raft/snapshot \
        > $BACKUP_DIR/openbao-snapshot-$DATE.bin
fi

# Keep 30 days of backups
find $BACKUP_DIR -type f -mtime +30 -delete
```

### Restore Procedure

```bash
# PostgreSQL
sudo -u postgres psql novanas < /mnt/backups/novanas-db-YYYYMMDD.sql

# OpenBao (requires stopping the service)
sudo systemctl stop openbao.service
sudo -u openbao /usr/bin/bao storage raft restore /mnt/backups/openbao-snapshot-*.bin
sudo systemctl start openbao.service
```

## TLS Configuration

### Using Let's Encrypt with OpenBao PKI

Set up an internal PKI and use OpenBao's PKI engine to issue certificates:

```bash
# Enable PKI backend
curl -sk -X POST \
    -H "X-Vault-Token: <root-token>" \
    https://localhost:8200/v1/sys/mounts/pki_root \
    -d '{"type": "pki"}'

# Create root CA
curl -sk -X POST \
    -H "X-Vault-Token: <root-token>" \
    https://localhost:8200/v1/pki_root/root/generate/internal \
    -d '{
        "common_name": "novanas.local",
        "ttl": "87600h"
    }'

# Enable intermediate PKI
curl -sk -X POST \
    -H "X-Vault-Token: <root-token>" \
    https://localhost:8200/v1/sys/mounts/pki_int \
    -d '{"type": "pki"}'

# For production, integrate with cert-manager on k3s for automated rotation
```

## k3s Integration

### Installing k3s

Once the infrastructure is stable:

```bash
# Install k3s
curl -sfL https://get.k3s.io | sh -

# Configure to use NovaNAS storage (example)
# See k3s documentation for persistent volume setup
```

### Accessing OpenBao from k3s

Install Vault Secrets Operator to automatically sync secrets:

```bash
helm repo add hashicorp https://helm.releases.hashicorp.com
helm install vault-secrets-operator hashicorp/vault-secrets-operator \
    --namespace vault-secrets-operator-system --create-namespace

# Create SecretStore CRD pointing to OpenBao
kubectl apply -f - << 'EOF'
apiVersion: secrets.hashicorp.com/v1beta1
kind: VaultStaticSecret
metadata:
  name: novanas-secrets
spec:
  vaultAuthPath: "kubernetes"
  vaultSecretPath: "secret/data/novanas/*"
EOF
```

## Troubleshooting

### OpenBao stuck unsealed

Check logs and unseal manually:

```bash
sudo journalctl -u openbao.service -n 50
sudo journalctl -u nova-bao-tpm-unseal.service -n 50

# Manual unseal (if TPM fails)
UNSEAL_KEYS=$(cat /run/openbao/unseal-keys.txt)  # Save these during setup
for key in $UNSEAL_KEYS; do
    curl -sk -X POST https://localhost:8200/v1/sys/unseal \
        -d "{\"key\": \"$key\"}"
done
```

### Keycloak won't start

```bash
# Check database connectivity
sudo -u keycloak psql -h localhost -U keycloak -d keycloak -c "SELECT 1"

# Check logs
sudo journalctl -u keycloak.service -n 50

# Verify environment
sudo cat /run/keycloak/keycloak.env
```

### NovaNAS API fails with auth errors

```bash
# Verify Keycloak is running
curl -sk http://localhost:8080/realms/novanas/.well-known/openid-configuration

# Check nova-api config
sudo cat /etc/nova-api/env

# Verify nova-api has access to OpenBao token
sudo cat /etc/nova-api/bao-token
```

### PCR Mismatch on Boot

If nova-bao-tpm-unseal fails with "PCR mismatch":

1. **Cause**: Boot state changed (BIOS update, secure boot toggle, kernel upgrade)
2. **Fix**: Manually unseal and re-seal:
   ```bash
   # Stop openbao
   sudo systemctl stop openbao.service
   
   # Unseal manually with Shamir keys (saved during setup)
   /usr/bin/bao operator unseal <key1>
   /usr/bin/bao operator unseal <key2>
   /usr/bin/bao operator unseal <key3>
   
   # Re-seal via TPM (regenerates PCR binding)
   sudo /usr/local/bin/nova-bao-unseal --init < <(jq -s . <<< "$(echo <keys> | tr ' ' '\n')")
   ```

## Performance Tuning

### PostgreSQL

For moderate workloads (default Debian config is safe):

```sql
ALTER SYSTEM SET shared_buffers = '4GB';
ALTER SYSTEM SET effective_cache_size = '12GB';
ALTER SYSTEM SET maintenance_work_mem = '1GB';
ALTER SYSTEM SET checkpoint_completion_target = 0.9;
ALTER SYSTEM SET wal_buffers = '16MB';
SELECT pg_reload_conf();
```

### Redis

```bash
# /etc/redis/redis.conf
maxmemory 4gb
maxmemory-policy allkeys-lru
```

### Keycloak

```bash
# /opt/keycloak/conf/keycloak.conf
db-pool-initial-size=10
db-pool-max-size=20
```

## Security Hardening

1. **Firewall**: Restrict port 8200 (OpenBao) and 8080 (Keycloak) to trusted networks
2. **TLS**: Replace self-signed certs with proper CA certificates
3. **RBAC**: Create fine-grained OpenBao policies per service
4. **Secrets**: Rotate service account tokens every 90 days
5. **Audit**: Enable audit logging on PostgreSQL and OpenBao
6. **SELinux**: Configure if using Fedora/RHEL

## References

- [OpenBao Documentation](https://openbao.org)
- [Keycloak Documentation](https://www.keycloak.org)
- [NovaNAS Architecture](../superpowers/specs/2026-04-28-novanas-storage-mvp-design.md)

## Support

For issues or questions, check:
1. Systemd journal: `journalctl -u <service> -n 50`
2. Application logs in `/var/log/`
3. This guide's Troubleshooting section
