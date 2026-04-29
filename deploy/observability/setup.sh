#!/bin/bash
# setup.sh: Complete observability stack installation (Prometheus, Alertmanager, Grafana, Loki, Promtail)
# Prerequisites:
#   - /etc/nova-ca/ca.crt exists (local CA)
#   - ZFS datasets created: sudo zfs create tank/system/{prometheus,alertmanager,grafana,loki,promtail}
#   - Grafana Labs apt repo enabled for loki/promtail
#
# Usage: sudo bash setup.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors
RED='\033[0;31m'
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
    exit 1
}

# Check if running as root
if [[ $EUID -ne 0 ]]; then
    error "This script must be run as root"
fi

# Verify CA exists
if [[ ! -f /etc/nova-ca/ca.crt ]]; then
    error "CA not found at /etc/nova-ca/ca.crt"
fi

log "Starting observability stack installation..."

# 1. Issue TLS certs from local CA
log "Step 1: Issuing TLS certificates..."
bash "$SCRIPT_DIR/observability/issue-certs.sh" || error "Failed to issue certs"

# 2. Install Debian packages
log "Step 2: Installing observability packages..."
apt-get update || error "apt-get update failed"

# Install from standard repos
apt-get install -y \
    prometheus \
    prometheus-alertmanager \
    grafana \
    prometheus-node-exporter \
    || error "Failed to install standard packages"

# Install Loki and Promtail from Grafana Labs repo
if ! apt-cache search loki 2>/dev/null | grep -q "^loki "; then
    warn "Loki not in default repos, adding Grafana Labs repo..."
    apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 12D524F46EE5156B || true
    echo "deb https://apt.grafana.com stable main" > /etc/apt/sources.list.d/grafana.list
    apt-get update || warn "Failed to add Grafana repo"
fi

apt-get install -y loki promtail || error "Failed to install loki/promtail"

# 3. Create system users
log "Step 3: Creating system users..."
for user in prometheus alertmanager grafana loki; do
    if ! id "$user" &>/dev/null; then
        useradd --system --home "/var/lib/$user" --shell /bin/false "$user" || warn "User $user may already exist"
    fi
done

# promtail runs as root (needs journald access)

# 4. Create ZFS dataset mountpoints
log "Step 4: Creating ZFS datasets and mount points..."
for comp in prometheus alertmanager grafana loki promtail; do
    dataset="tank/system/$comp"
    mountpoint="/var/lib/$comp"

    # Create dataset if not exists
    if ! zfs list "$dataset" >/dev/null 2>&1; then
        log "Creating ZFS dataset $dataset..."
        zfs create "$dataset" || error "Failed to create $dataset"
    else
        log "ZFS dataset $dataset already exists"
    fi

    # Verify mountpoint
    if [[ ! -d "$mountpoint" ]]; then
        mkdir -p "$mountpoint"
    fi

    # Set ownership (except promtail which is root)
    if [[ "$comp" != "promtail" ]]; then
        chown "$comp:$comp" "$mountpoint"
        chmod 0750 "$mountpoint"
    else
        chown root:root "$mountpoint"
        chmod 0755 "$mountpoint"
    fi
done

# 5. Create config directories
log "Step 5: Creating config directories..."
for dir in /etc/{prometheus,alertmanager,grafana,loki,promtail}; do
    mkdir -p "$dir"
done

# 6. Copy config files
log "Step 6: Copying configuration files..."
cp "$SCRIPT_DIR/prometheus/prometheus.yml" /etc/prometheus/
chmod 0644 /etc/prometheus/prometheus.yml

cp "$SCRIPT_DIR/alertmanager/alertmanager.yml" /etc/alertmanager/
chmod 0644 /etc/alertmanager/alertmanager.yml

cp "$SCRIPT_DIR/grafana/grafana.ini" /etc/grafana/
chmod 0644 /etc/grafana/grafana.ini

# Create Grafana provisioning dirs
mkdir -p /etc/grafana/provisioning/{datasources,dashboards}
cp "$SCRIPT_DIR/grafana/provisioning/datasources/"*.yaml /etc/grafana/provisioning/datasources/
cp "$SCRIPT_DIR/grafana/provisioning/dashboards/dashboard.yaml" /etc/grafana/provisioning/dashboards/

# Create Grafana dashboards dir
mkdir -p /etc/grafana/dashboards
cp "$SCRIPT_DIR/grafana/dashboards/"*.json /etc/grafana/dashboards/
chmod 0644 /etc/grafana/dashboards/*.json

cp "$SCRIPT_DIR/loki/loki-config.yaml" /etc/loki/
chmod 0644 /etc/loki/loki-config.yaml

cp "$SCRIPT_DIR/promtail/promtail-config.yaml" /etc/promtail/
chmod 0644 /etc/promtail/promtail-config.yaml

# 7. Copy systemd units
log "Step 7: Installing systemd units..."
cp "$SCRIPT_DIR/systemd/prometheus.service" /etc/systemd/system/
cp "$SCRIPT_DIR/systemd/alertmanager.service" /etc/systemd/system/
cp "$SCRIPT_DIR/systemd/grafana.service" /etc/systemd/system/
cp "$SCRIPT_DIR/systemd/loki.service" /etc/systemd/system/
cp "$SCRIPT_DIR/systemd/promtail.service" /etc/systemd/system/

chmod 0644 /etc/systemd/system/{prometheus,alertmanager,grafana,loki,promtail}.service

systemctl daemon-reload

# 8. Create env files for services (operators can customize)
log "Step 8: Creating environment files..."
cat > /etc/prometheus/prometheus.env << 'EOF'
# Prometheus environment variables
# Operators can add secrets from OpenBao here
EOF
chmod 0644 /etc/prometheus/prometheus.env

cat > /etc/alertmanager/alertmanager.env << 'EOF'
# Alertmanager environment variables
# Example: BAO_ADDR=https://localhost:8200
EOF
chmod 0644 /etc/alertmanager/alertmanager.env

cat > /etc/grafana/grafana.env << 'EOF'
# Grafana environment variables
# Example: GF_SECURITY_ADMIN_PASSWORD={{ SECRET:grafana/admin_pass }}
EOF
chmod 0644 /etc/grafana/grafana.env

cat > /etc/loki/loki.env << 'EOF'
# Loki environment variables
EOF
chmod 0644 /etc/loki/loki.env

cat > /etc/promtail/promtail.env << 'EOF'
# Promtail environment variables
EOF
chmod 0644 /etc/promtail/promtail.env

# 9. Fix permissions on config dirs
log "Step 9: Setting permissions..."
for dir in /etc/{prometheus,alertmanager,grafana,loki,promtail}; do
    if [[ "$dir" == "/etc/grafana" ]]; then
        chown -R grafana:grafana "$dir"
        chmod 0750 "$dir"
    elif [[ "$dir" == "/etc/prometheus" ]]; then
        chown -R prometheus:prometheus "$dir"
        chmod 0750 "$dir"
    elif [[ "$dir" == "/etc/alertmanager" ]]; then
        chown -R alertmanager:alertmanager "$dir"
        chmod 0750 "$dir"
    elif [[ "$dir" == "/etc/loki" ]]; then
        chown -R loki:loki "$dir"
        chmod 0750 "$dir"
    elif [[ "$dir" == "/etc/promtail" ]]; then
        chown -R root:root "$dir"
        chmod 0755 "$dir"
    fi
done

# 10. Create rules directory for Prometheus
log "Step 10: Creating Prometheus rules directory..."
mkdir -p /etc/prometheus/rules
chmod 0755 /etc/prometheus/rules
chown prometheus:prometheus /etc/prometheus/rules

# Create placeholder rules file
cat > /etc/prometheus/rules/novanas.rules.yml << 'EOF'
groups:
  - name: novanas
    interval: 30s
    rules:
      # Example rule: high memory usage
      # - alert: HighMemoryUsage
      #   expr: (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) < 0.1
      #   for: 5m
      #   labels:
      #     severity: warning
      #   annotations:
      #     summary: High memory usage on {{ $labels.instance }}
EOF
chmod 0644 /etc/prometheus/rules/novanas.rules.yml

# 11. Copy certs to component directories
log "Step 11: Installing TLS certificates..."
for comp in prometheus alertmanager grafana; do
    cp /etc/nova-certs/${comp}.crt "/etc/${comp}/"
    cp /etc/nova-certs/${comp}.key "/etc/${comp}/"
    chown "$comp:$comp" "/etc/${comp}/${comp}.crt" "/etc/${comp}/${comp}.key"
    chmod 0600 "/etc/${comp}/${comp}.key"
    chmod 0644 "/etc/${comp}/${comp}.crt"
done

# 12. Initialize Grafana database
log "Step 12: Initializing Grafana database..."
mkdir -p /var/lib/grafana
chown grafana:grafana /var/lib/grafana
chmod 0750 /var/lib/grafana

# 13. Enable units (but don't start yet)
log "Step 13: Enabling systemd units..."
systemctl enable prometheus.service
systemctl enable alertmanager.service
systemctl enable grafana.service
systemctl enable loki.service
systemctl enable promtail.service

# 14. Keycloak SSO bootstrap (Grafana OIDC + oauth2-proxy fronts).
#     Idempotent: re-running rotates client secrets and refreshes
#     cookie secrets. Requires KC_URL and KC_ADMIN_PASS in the env.
log "Step 14: Bootstrapping Keycloak SSO clients..."
if [[ -n "${KC_URL:-}" && -n "${KC_ADMIN_PASS:-}" ]]; then
    # Create oauth2-proxy system user/group + config dir.
    if ! id oauth2-proxy &>/dev/null; then
        useradd --system --home /var/lib/oauth2-proxy --shell /bin/false oauth2-proxy \
            || warn "could not create oauth2-proxy user"
    fi
    mkdir -p /etc/oauth2-proxy/tls
    chown -R oauth2-proxy:oauth2-proxy /etc/oauth2-proxy
    chmod 0750 /etc/oauth2-proxy
    chmod 0750 /etc/oauth2-proxy/tls

    # Drop oauth2-proxy configs in place.
    for svc in prometheus alertmanager loki; do
        cp "$SCRIPT_DIR/oauth2-proxy/${svc}.cfg" "/etc/oauth2-proxy/${svc}.cfg"
        chown oauth2-proxy:oauth2-proxy "/etc/oauth2-proxy/${svc}.cfg"
        chmod 0640 "/etc/oauth2-proxy/${svc}.cfg"
        # Empty env override file (operators may add overrides).
        if [[ ! -f "/etc/oauth2-proxy/${svc}.env" ]]; then
            : > "/etc/oauth2-proxy/${svc}.env"
            chown oauth2-proxy:oauth2-proxy "/etc/oauth2-proxy/${svc}.env"
            chmod 0640 "/etc/oauth2-proxy/${svc}.env"
        fi
        # Reuse host certs (CA-signed) for oauth2-proxy TLS.
        if [[ -f "/etc/nova-certs/${svc}.crt" && -f "/etc/nova-certs/${svc}.key" ]]; then
            cp "/etc/nova-certs/${svc}.crt" "/etc/oauth2-proxy/tls/${svc}.crt"
            cp "/etc/nova-certs/${svc}.key" "/etc/oauth2-proxy/tls/${svc}.key"
            chown oauth2-proxy:oauth2-proxy "/etc/oauth2-proxy/tls/${svc}.crt" "/etc/oauth2-proxy/tls/${svc}.key"
            chmod 0644 "/etc/oauth2-proxy/tls/${svc}.crt"
            chmod 0600 "/etc/oauth2-proxy/tls/${svc}.key"
        else
            warn "oauth2-proxy TLS material missing for $svc; issue-certs.sh must publish ${svc}.crt/.key"
        fi
    done

    # Grafana OIDC client.
    log "  Creating grafana Keycloak client..."
    GRAFANA_JSON="$(bash "$SCRIPT_DIR/keycloak/create-grafana-client.sh")" \
        || error "create-grafana-client.sh failed"
    GRAFANA_SECRET="$(jq -r '.clientSecret' <<<"$GRAFANA_JSON")"
    if [[ -z "$GRAFANA_SECRET" || "$GRAFANA_SECRET" == "null" ]]; then
        error "did not receive grafana client secret"
    fi
    install -m 0400 -o grafana -g grafana /dev/null /etc/grafana/oidc-secret
    printf '%s' "$GRAFANA_SECRET" > /etc/grafana/oidc-secret
    chown grafana:grafana /etc/grafana/oidc-secret
    chmod 0400 /etc/grafana/oidc-secret

    # oauth2-proxy clients (one per protected service).
    log "  Creating oauth2-proxy Keycloak clients..."
    O2P_JSON="$(bash "$SCRIPT_DIR/keycloak/create-oauth2-proxy-clients.sh")" \
        || error "create-oauth2-proxy-clients.sh failed"

    for svc in prometheus alertmanager loki; do
        cs="$(jq -r --arg s "$svc" '.clients[]|select(.service==$s)|.clientSecret' <<<"$O2P_JSON")"
        ck="$(jq -r --arg s "$svc" '.clients[]|select(.service==$s)|.cookieSecret' <<<"$O2P_JSON")"
        if [[ -z "$cs" || "$cs" == "null" || -z "$ck" || "$ck" == "null" ]]; then
            error "missing secrets for oauth2-proxy-$svc"
        fi
        printf '%s' "$cs" > "/etc/oauth2-proxy/${svc}-client-secret"
        printf '%s' "$ck" > "/etc/oauth2-proxy/${svc}-cookie-secret"
        chown oauth2-proxy:oauth2-proxy \
            "/etc/oauth2-proxy/${svc}-client-secret" \
            "/etc/oauth2-proxy/${svc}-cookie-secret"
        chmod 0400 \
            "/etc/oauth2-proxy/${svc}-client-secret" \
            "/etc/oauth2-proxy/${svc}-cookie-secret"
    done

    # oauth2-proxy systemd units.
    cp "$SCRIPT_DIR/systemd/oauth2-proxy-prometheus.service" /etc/systemd/system/
    cp "$SCRIPT_DIR/systemd/oauth2-proxy-alertmanager.service" /etc/systemd/system/
    cp "$SCRIPT_DIR/systemd/oauth2-proxy-loki.service" /etc/systemd/system/
    chmod 0644 /etc/systemd/system/oauth2-proxy-{prometheus,alertmanager,loki}.service
    systemctl daemon-reload
    systemctl enable oauth2-proxy-prometheus.service
    systemctl enable oauth2-proxy-alertmanager.service
    systemctl enable oauth2-proxy-loki.service

    # Restart Grafana so the new OIDC config (and secret file) takes
    # effect, but only if it's already running.
    if systemctl is-active --quiet grafana.service; then
        systemctl restart grafana.service || warn "grafana restart failed"
    fi
else
    warn "KC_URL/KC_ADMIN_PASS not set; skipping SSO bootstrap. See docs/observability/sso.md."
fi

log "✓ Installation complete!"
log ""
log "Next steps:"
log "1. Review and customize config files:"
log "   - /etc/prometheus/prometheus.yml"
log "   - /etc/alertmanager/alertmanager.yml"
log "   - /etc/grafana/grafana.ini"
log "   - /etc/loki/loki-config.yaml"
log "   - /etc/promtail/promtail-config.yaml"
log ""
log "2. Set admin password in /etc/grafana/grafana.ini (or via OpenBao secret)"
log ""
log "3. Start services in order (wait for Loki before Promtail):"
log "   sudo systemctl start prometheus.service"
log "   sudo systemctl start alertmanager.service"
log "   sudo systemctl start loki.service"
log "   sudo systemctl start promtail.service"
log "   sudo systemctl start grafana.service"
log ""
log "4. Verify:"
log "   curl -sk https://127.0.0.1:9090/api/v1/status/config"
log "   curl -sk https://127.0.0.1:9093/-/healthy"
log "   curl -sk https://127.0.0.1:3000/api/health"
log "   curl -s http://127.0.0.1:3100/ready"
log ""
log "5. SSO (Keycloak):"
log "   - With KC_URL + KC_ADMIN_PASS exported, this script provisioned"
log "     Grafana OIDC + oauth2-proxy clients in realm 'novanas'."
log "   - Public URLs once oauth2-proxy units are started:"
log "       https://novanas.local:3000   (Grafana, native OIDC)"
log "       https://novanas.local:9091   (Prometheus via oauth2-proxy)"
log "       https://novanas.local:9094   (Alertmanager via oauth2-proxy)"
log "       https://novanas.local:3101   (Loki via oauth2-proxy)"
log "   - Start the oauth2-proxy units:"
log "       sudo systemctl start oauth2-proxy-prometheus.service"
log "       sudo systemctl start oauth2-proxy-alertmanager.service"
log "       sudo systemctl start oauth2-proxy-loki.service"
log "   - See docs/observability/sso.md for troubleshooting."
