# NovaNAS Observability Stack

Complete monitoring, alerting, and logging for NovaNAS infrastructure running on systemd (not k3s).

## Overview

The observability stack provides:

- **Prometheus**: Metrics collection from all NovaNAS components
- **Alertmanager**: Alert routing and management
- **Grafana**: Dashboards and visualization
- **Loki**: Log aggregation from journald and container logs
- **Promtail**: Log shipping to Loki

All components:
- Run as dedicated system users
- Store data in ZFS datasets under `/var/lib/<component>`
- Use self-signed TLS certs signed by the local CA at `/etc/nova-ca/ca.crt`
- Listen on localhost/management addresses only
- Can optionally read credentials from OpenBao at startup

### Component Ports

| Component | Port | Protocol | Purpose |
|-----------|------|----------|---------|
| Prometheus | 9090 | HTTPS | Metrics API & UI |
| Alertmanager | 9093 | HTTPS | Alert routing & API |
| Grafana | 3000 | HTTPS | Dashboards & visualization |
| Loki | 3100 | HTTP | Log API (no TLS by default) |
| Promtail | (no listen) | - | Ships logs via push to Loki |

### Architecture Diagram

```
NovaNAS Infrastructure
├─ Prometheus (9090 HTTPS)
│  ├─ nova-api /metrics
│  ├─ node-exporter (9100)
│  ├─ postgres-exporter (9187)
│  ├─ redis-exporter (9121)
│  └─ k3s kubelet/etcd/kube-proxy (10250, 2379, 10249)
├─ Alertmanager (9093 HTTPS)
│  └─ Rule evaluation + notification routing
├─ Grafana (3000 HTTPS)
│  ├─ Prometheus datasource
│  └─ Loki datasource
├─ Loki (3100 HTTP)
│  └─ Filesystem storage (/var/lib/loki)
└─ Promtail (journald shipper)
   ├─ journald → Loki
   └─ /var/lib/rancher/k3s container logs → Loki
```

## Installation

### Prerequisites

1. **Local CA**: `/etc/nova-ca/ca.crt` must exist (created during OpenBao setup)
2. **ZFS datasets**: Create before running setup (or setup.sh will attempt to create them):
   ```bash
   sudo zfs create tank/system/prometheus
   sudo zfs create tank/system/alertmanager
   sudo zfs create tank/system/grafana
   sudo zfs create tank/system/loki
   sudo zfs create tank/system/promtail
   ```
3. **Grafana Labs apt repo** (for loki/promtail):
   ```bash
   apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 12D524F46EE5156B
   echo "deb https://apt.grafana.com stable main" | sudo tee /etc/apt/sources.list.d/grafana.list
   ```

### Automated Installation

From the NovaNAS repo root:

```bash
sudo bash deploy/observability/setup.sh
```

This script:
- Issues TLS certs from the local CA
- Installs Debian packages (prometheus, grafana, loki, promtail, etc.)
- Creates system users and ZFS mountpoints
- Copies config files to `/etc/<component>/`
- Enables systemd units

### Manual Installation (if not using setup.sh)

1. Install packages:
   ```bash
   sudo apt-get update
   sudo apt-get install -y prometheus prometheus-alertmanager grafana loki promtail prometheus-node-exporter
   ```

2. Copy systemd units:
   ```bash
   sudo cp deploy/systemd/{prometheus,alertmanager,grafana,loki,promtail}.service /etc/systemd/system/
   sudo systemctl daemon-reload
   ```

3. Copy config files:
   ```bash
   sudo cp deploy/prometheus/prometheus.yml /etc/prometheus/
   sudo cp deploy/alertmanager/alertmanager.yml /etc/alertmanager/
   sudo cp deploy/grafana/grafana.ini /etc/grafana/
   sudo cp deploy/loki/loki-config.yaml /etc/loki/
   sudo cp deploy/promtail/promtail-config.yaml /etc/promtail/
   ```

4. Issue TLS certs:
   ```bash
   bash deploy/observability/issue-certs.sh
   ```

5. Enable and start:
   ```bash
   sudo systemctl enable {prometheus,alertmanager,loki,promtail,grafana}.service
   sudo systemctl start prometheus alertmanager loki promtail grafana
   ```

## Configuration

### Prometheus

**Config file**: `/etc/prometheus/prometheus.yml`

Key sections:

- **`global`**: Scrape interval, evaluation interval, external labels
- **`scrape_configs`**: Target definitions
  - **nova-api** (`/metrics` endpoint): Static HTTPS target with CA verification
  - **node-exporter** (9100): Local node metrics
  - **postgres-exporter** (9187): Database metrics
  - **redis-exporter** (9121): Cache metrics
  - **k3s** kubelet/etcd/kube-proxy: HTTPS targets with authorization
  - **kube-state-metrics**: Kubernetes object metrics (discovered via kubernetes_sd_config)
- **`alerting`**: Alertmanager target (localhost:9093)
- **`rule_files`**: Alert rules from `/etc/prometheus/rules/*.yml`

#### Adding Custom Scrape Targets

Edit `/etc/prometheus/prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'my-service'
    static_configs:
      - targets: ['127.0.0.1:9999']
    scrape_interval: 30s
```

Then reload:

```bash
curl -X POST http://127.0.0.1:9090/-/reload
```

#### Adding Alert Rules

Create rule files in `/etc/prometheus/rules/`:

```bash
sudo cat > /etc/prometheus/rules/custom.rules.yml << 'EOF'
groups:
  - name: custom
    rules:
      - alert: MyAlert
        expr: up{job="my-service"} == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "{{ $labels.job }} is down"
EOF

# Reload Prometheus
curl -X POST http://127.0.0.1:9090/-/reload
```

### Alertmanager

**Config file**: `/etc/alertmanager/alertmanager.yml`

Key sections:

- **`global`**: SMTP settings (operators can add from OpenBao)
- **`route`**: Alert routing tree (default: null receiver)
- **`receivers`**: Notification destinations (email, Slack, webhook, etc.)
- **`inhibit_rules`**: Rules to suppress alerts

#### Enabling SMTP Notifications

Example: Add email receiver to `/etc/alertmanager/alertmanager.yml`:

```yaml
global:
  smtp_smarthost: 'smtp.example.com:587'
  smtp_auth_username: 'alerts@example.com'
  smtp_auth_password: '{{ SECRET:alertmanager/smtp_pass }}'
  smtp_from: 'alerts@novanas.local'

receivers:
  - name: 'email'
    email_configs:
      - to: 'ops@example.com'
        send_resolved: true

route:
  receiver: 'email'
  routes:
    - match_re:
        severity: 'critical'
      receiver: 'email'
```

Reload:

```bash
curl -X POST http://127.0.0.1:9093/-/reload
```

### Grafana

**Config file**: `/etc/grafana/grafana.ini`

Default login: `admin` (password in `/etc/grafana/grafana.ini` or OpenBao secret)

#### Keycloak OIDC Integration

To integrate with Keycloak for single sign-on:

1. Create a new client in Keycloak:
   - Name: `grafana`
   - Client ID: `grafana`
   - Client authentication: enabled
   - Redirect URIs: `https://novanas.local:3000/login/generic_oauth`
   - Save the client secret

2. Store secret in OpenBao:
   ```bash
   bao kv put secret/observability/grafana \
     oauth_secret="<client-secret>" \
     admin_pass="<admin-password>"
   ```

3. Update `/etc/grafana/grafana.ini`:
   ```ini
   [auth.generic_oauth]
   enabled = true
   name = Keycloak
   client_id = grafana
   client_secret = {{ SECRET:grafana/oauth_secret }}
   scopes = openid profile email roles
   auth_url = https://192.168.10.204:8443/realms/novanas/protocol/openid-connect/auth
   token_url = https://192.168.10.204:8443/realms/novanas/protocol/openid-connect/token
   api_url = https://192.168.10.204:8443/realms/novanas/protocol/openid-connect/userinfo
   role_attribute_path = contains(realm_access.roles[*], 'nova-admin') && 'Admin' || contains(realm_access.roles[*], 'nova-operator') && 'Editor' || 'Viewer'
   allow_sign_up = true
   ```

4. Restart Grafana:
   ```bash
   sudo systemctl restart grafana.service
   ```

#### Adding Custom Dashboards

1. Create a JSON dashboard file in `/etc/grafana/dashboards/`:
   ```bash
   sudo cp my-dashboard.json /etc/grafana/dashboards/
   sudo chown grafana:grafana /etc/grafana/dashboards/my-dashboard.json
   ```

2. Grafana auto-discovers dashboards in the provisioning folder (30s scan interval)

Alternatively, import via Grafana UI at `https://novanas.local:3000/dashboard/import`

### Loki

**Config file**: `/etc/loki/loki-config.yaml`

Key settings:

- **`ingester`**: Memory limits and chunk configuration
- **`limits_config`**: Rate limits, retention period (30d default)
- **`schema_config`**: Storage backend (filesystem + boltdb-shipper)
- **`storage_config`**: Filesystem directory `/var/lib/loki`
- **`compactor`**: Retention enforcement and compaction

#### Changing Retention

Edit `/etc/loki/loki-config.yaml`:

```yaml
limits_config:
  retention_period: 1440h  # 60 days
```

Reload:

```bash
systemctl reload loki.service
```

#### Using S3 for Long-term Storage

(Optional) Configure S3 backend:

```yaml
storage_config:
  s3:
    endpoint: 's3.amazonaws.com'
    region: 'us-east-1'
    bucket_name: 'novanas-logs'
    access_key_id: '{{ SECRET:observability/loki/s3_access_key }}'
    secret_access_key: '{{ SECRET:observability/loki/s3_secret_key }}'
```

### Promtail

**Config file**: `/etc/promtail/promtail-config.yaml`

Scrape configs:

- **journald**: Reads systemd journal, labels services by unit name
- **kubernetes**: Reads k3s container logs from `/var/lib/rancher/k3s/agent/containerd/io.containerd.runtime.v2.task/k8s.io/*/log.json`

Logs are pushed to Loki at `http://127.0.0.1:3100/loki/api/v1/push`

#### Adding Custom Log Sources

Edit `/etc/promtail/promtail-config.yaml` and add under `scrape_configs`:

```yaml
- job_name: custom_logs
  static_configs:
    - targets:
        - localhost
      labels:
        job: my-app
        __path__: /var/log/my-app/*.log
  relabel_configs:
    - source_labels: ['__path__']
      regex: '.*/(.*)\.log'
      target_label: 'logfile'
```

Reload:

```bash
sudo systemctl reload promtail.service
```

## Dashboards

Pre-built dashboards available in Grafana:

- **NovaNAS Storage**: ZFS pool capacity, vdev errors, fragmentation
- **NovaNAS API**: HTTP RPS by route, latency p50/p95/p99, error rate, auth events
- **NovaNAS Jobs**: Job dispatch rate, in-flight gauge, duration p95
- **NovaNAS Logs**: Recent logs from nova-api, openbao, keycloak, and system services

Dashboards are auto-loaded from `/etc/grafana/dashboards/` on Grafana start.

## Operational Tasks

### Starting Services

Boot order (wait for each to be ready):

```bash
# 1. Prometheus (needs to be healthy first)
sudo systemctl start prometheus.service
sleep 10

# 2. Alertmanager
sudo systemctl start alertmanager.service
sleep 5

# 3. Loki (Promtail depends on this)
sudo systemctl start loki.service
sleep 5

# 4. Promtail (depends on Loki)
sudo systemctl start promtail.service
sleep 5

# 5. Grafana
sudo systemctl start grafana.service
```

### Checking Status

```bash
# Prometheus
curl -sk https://127.0.0.1:9090/api/v1/status/config

# Alertmanager
curl -sk https://127.0.0.1:9093/-/healthy

# Grafana
curl -sk https://127.0.0.1:3000/api/health

# Loki
curl -s http://127.0.0.1:3100/ready

# Promtail
sudo journalctl -u promtail.service -n 20
```

### Monitoring Logs

Real-time logs:

```bash
sudo journalctl -u prometheus.service -f
sudo journalctl -u alertmanager.service -f
sudo journalctl -u grafana.service -f
sudo journalctl -u loki.service -f
sudo journalctl -u promtail.service -f
```

### Reloading Configs

Most configs can be reloaded without restart:

```bash
# Prometheus (must be running)
curl -X POST http://127.0.0.1:9090/-/reload

# Alertmanager (must be running)
curl -X POST http://127.0.0.1:9093/-/reload

# Loki, Promtail, Grafana require restart
sudo systemctl restart loki.service
sudo systemctl restart promtail.service
sudo systemctl restart grafana.service
```

### Backing Up Data

#### Prometheus

Prometheus stores time-series data in `/var/lib/prometheus/`. For backups:

```bash
# Stop Prometheus
sudo systemctl stop prometheus.service

# Backup data directory
sudo tar -czf /mnt/backups/prometheus-$(date +%Y%m%d).tar.gz /var/lib/prometheus/

# Start Prometheus
sudo systemctl start prometheus.service
```

#### Grafana

Grafana uses SQLite at `/var/lib/grafana/grafana.db`:

```bash
sudo cp /var/lib/grafana/grafana.db /mnt/backups/grafana-$(date +%Y%m%d).db
```

#### Loki

Loki stores chunks and index in `/var/lib/loki/`:

```bash
sudo tar -czf /mnt/backups/loki-$(date +%Y%m%d).tar.gz /var/lib/loki/
```

### Disaster Recovery

#### Restore Prometheus

```bash
# Stop service
sudo systemctl stop prometheus.service

# Restore data
sudo tar -xzf /mnt/backups/prometheus-YYYYMMDD.tar.gz -C /

# Fix permissions
sudo chown -R prometheus:prometheus /var/lib/prometheus

# Start
sudo systemctl start prometheus.service
```

#### Restore Grafana

```bash
# Stop service
sudo systemctl stop grafana.service

# Restore DB
sudo cp /mnt/backups/grafana-YYYYMMDD.db /var/lib/grafana/grafana.db
sudo chown grafana:grafana /var/lib/grafana/grafana.db

# Start
sudo systemctl start grafana.service
```

#### Restore Loki

```bash
# Stop service
sudo systemctl stop loki.service

# Restore data
sudo tar -xzf /mnt/backups/loki-YYYYMMDD.tar.gz -C /

# Fix permissions
sudo chown -R loki:loki /var/lib/loki

# Start
sudo systemctl start loki.service
```

## Troubleshooting

### Prometheus won't start

Check logs:

```bash
sudo journalctl -u prometheus.service -n 50

# Check config syntax
prometheus --config.file=/etc/prometheus/prometheus.yml --syntax-only
```

Common issues:

- Bad YAML in config files
- File permissions on `/var/lib/prometheus`
- Port 9090 already in use

### Alertmanager won't start

```bash
sudo journalctl -u alertmanager.service -n 50

# Check config syntax
alertmanager --config.file=/etc/alertmanager/alertmanager.yml --check-config
```

### Grafana login fails

Default credentials:

- Username: `admin`
- Password: Check `/etc/grafana/grafana.ini` or environment variable `GF_SECURITY_ADMIN_PASSWORD`

If forgot password:

```bash
# Reset via Grafana CLI
sudo /usr/sbin/grafana-cli admin reset-admin-password <new-password>
```

### Loki full disk

Check data usage:

```bash
du -sh /var/lib/loki/*

# View retention setting
grep retention_period /etc/loki/loki-config.yaml
```

If full, either:
1. Increase retention and wait for compactor to run
2. Manually delete old chunks from `/var/lib/loki/chunks/`
3. Switch to external storage (S3, etc.)

### Promtail not shipping logs

Check connectivity:

```bash
# Verify Loki is running
curl -s http://127.0.0.1:3100/ready

# Check Promtail logs
sudo journalctl -u promtail.service -n 50

# Test journal access
sudo journalctl -n 5
```

If Promtail can't read journald, verify it's running as root:

```bash
systemctl status promtail.service | grep -i user
```

### High CPU/Memory usage

- **Prometheus**: Reduce scrape interval in `/etc/prometheus/prometheus.yml`, or reduce number of targets
- **Grafana**: Reduce dashboard update frequency, optimize queries
- **Loki**: Reduce ingestion rate or retention period

## Performance Tuning

### Prometheus

For high cardinality metrics, increase memory in systemd unit:

```ini
[Service]
Environment="GOMAXPROCS=4"
```

And raise file descriptors:

```ini
LimitNOFILE=65536
```

### Loki

For high log volume:

```yaml
limits_config:
  ingestion_rate_mb: 50      # Increase from 10
  ingestion_burst_size_mb: 100
```

### Grafana

Increase session timeout for long dashboards:

```ini
[session]
session_lifetime = 604800  # 1 week
```

## Security Hardening

1. **TLS**: All components use certs signed by local CA. Replace with proper certificates if exposing externally.
2. **Auth**: Grafana integrated with Keycloak via OIDC. Configure fine-grained role mappings.
3. **Network**: All components listen on localhost only. Use a reverse proxy or network policies if exposing.
4. **Secrets**: Store SMTP passwords, API keys, etc. in OpenBao, not in plaintext configs.
5. **RBAC**: Create OpenBao policies per component (see `deploy/openbao/*-policy.hcl`)

## References

- [Prometheus Documentation](https://prometheus.io/docs/)
- [Alertmanager Documentation](https://prometheus.io/docs/alerting/latest/overview/)
- [Grafana Documentation](https://grafana.com/docs/grafana/latest/)
- [Loki Documentation](https://grafana.com/docs/loki/latest/)
- [Promtail Documentation](https://grafana.com/docs/loki/latest/send-data/promtail/)

## Support

For issues:

1. Check logs: `sudo journalctl -u <service> -n 50`
2. Verify configs: Check YAML syntax and file paths
3. Test connectivity: `curl` health endpoints
4. Review this guide's Troubleshooting section
