# 12 — Observability

Three pillars, each with its own backend but unified via Grafana LGTM stack.

| Pillar | Backend | Retention | Consumer |
|---|---|---|---|
| **Metrics** | Prometheus | 90d+ native | Dashboards, alerts, SLOs |
| **Logs** | Loki | 30d hot / 90d cold | Troubleshooting, audit |
| **Traces** | Tempo | 7d (1% sampled) | Debugging |

Collected via **Grafana Alloy**. Dashboards + exploration via **Grafana**.

**No Mimir.** Prometheus long retention is sufficient at single-box scale; keeps the stack simple.

## Where observability data lives

- Prometheus short-term: persistent partition on OS disk (availability during storage issues)
- Long retention: Prometheus remote-write to a NovaNas S3 Bucket — self-hosted on the chunk engine (dog-food)
- Loki: S3 Bucket backend with local index on persistent partition
- Tempo: S3 Bucket backend

**Capacity budget** (20-disk NAS, ~30 apps):

| Data | Typical size |
|---|---|
| Prometheus (90d) | 10-30 GB |
| Loki (30d hot + 90d cold) | 50-300 GB |
| Tempo (7d, 1% sampled) | 5-50 GB |
| **Total** | **~0.5 TB worst case** |

Observability data uses a **low-priority protection class** (rep×2 on a warm pool, or EC 4+2 on cold) — regeneratable if lost.

## Metrics

### Scrape sources

| Target | Source |
|---|---|
| k3s / kubelet | Built-in `/metrics` |
| NovaStor agent, meta, csi, s3gw | Existing `internal/metrics` |
| novanet / novaedge | Own `/metrics` |
| NovaNas API | `prom-client` |
| Keycloak | Built-in `/metrics` |
| OpenBao | Built-in `/metrics` |
| Postgres | `postgres_exporter` sidecar |
| Redis | `redis_exporter` sidecar |
| Node (host, CPU, temp, SMART) | `node_exporter` + `smartctl_exporter` |
| Disks | NovaStor agent |
| App pods | Auto-scrape on `prometheus.io/scrape: true` annotation (default ON); opt-out per-app |
| VMs | KubeVirt exporter |

### NovaNas curated query gateway

NovaNas API does not forward raw PromQL from browsers. It exposes **typed endpoints** with pre-written queries:

```
GET /api/v1/metrics/pool/{name}/throughput?range=1h
GET /api/v1/metrics/disk/{wwn}/smart?range=7d
GET /api/v1/metrics/app/{ns}/{name}/resources?range=1h
GET /api/v1/metrics/vm/{ns}/{name}/cpu?range=24h
```

UI uses Recharts (dashboard sparklines) and ECharts (heavy detail views). Power users click "Open in Grafana" on any graph → deep-linked full Grafana dashboard with the same time range.

### Shipped Grafana dashboards

Pre-provisioned via ConfigMap; read-only for regular users; admins can clone and modify.

- **System Overview** — CPU, memory, disk, network, boot/uptime
- **Storage — Pools** — capacity, rebuild status, scrub status, IOPS, throughput
- **Storage — Disks** — SMART, per-disk IOPS/latency/errors
- **Storage — Datasets/Buckets** — capacity, growth, top-hot datasets
- **Protocols** — SMB sessions, NFS ops/s, iSCSI sessions, S3 RPS
- **Apps** — per-app resources, restart counts
- **VMs** — per-VM CPU/mem, virtio stats
- **Network** — per-interface bps/pps, VIP sessions
- **Cluster** — k3s / etcd health, API latency
- **Security** — failed logins, 2FA events, token issuances, admission rejections

Templatized by instance/pool/app so they work without config on any appliance.

### Embedded Grafana

- Deployed in `novanas-system`
- Auth: Keycloak SSO via Grafana's native OIDC integration
- Data sources pre-configured: Prometheus, Loki, Tempo, Postgres (for audit queries)
- Admins see all dashboards; viewers see their namespace-scoped views

## Logs

### Sources

- **k8s container logs** — Alloy tails `/var/log/pods/...` and ships to Loki
- **Host journald** — Alloy ships node logs (kernel, systemd, RAUC update logs)
- **Audit log** — NovaNas API writes to Postgres (indexed for fast UI queries) AND emits to Loki for long-term
- **Access logs** — SMB auth events, NFS mount events, S3 requests, UI access
- **Security events** — failed auth, firewall drops, admission rejections

### Structured logging

All NovaNas components log structured JSON. Common fields:

```
component         api | storage-meta | storage-agent | chunk-engine | filer | ...
request_id        correlates UI click → API → CRD write → reconcile
user              who initiated the request
session_id        browser session
resource_kind     Dataset, Pool, Disk, App, VM, ...
resource_name     specific target
severity          debug | info | warn | error
error.type        typed error classification (when applicable)
error.message     human-readable
trace_id          OpenTelemetry correlation
```

Libraries: pino (Node), zap (Go), tracing-subscriber JSON (Rust).

### Log viewer in UI

- Per-resource log panel — click on Dataset/Pool/App/VM/Disk, see last 500 lines + live tail via WS
- Global search with pre-built filter UI
- Advanced mode: raw LogQL for power users
- Audit log tab — queried from Postgres for the last 30 days (fast); Loki for older history

## Traces

- OpenTelemetry instrumentation in api, operators, storage gRPC clients/servers, chunk engine, dataplane
- Default sample rate 1% (bounded overhead)
- UI toggle: "debug window — 100% sampling for 60s" — for deliberate investigation
- Most useful for diagnosing slow operations that cross layers (UI click → API → CRD → operator → gRPC → chunk engine → disk)

## Alerting

**Prometheus Alertmanager** under the hood; admin-facing CRDs provide nicer UX.

### AlertChannel

```yaml
kind: AlertChannel
metadata: { name: admin-email }
spec:
  type: email              # email | webhook | ntfy | pushover | slack | telegram | teams
  recipients: [admin@example.com]
  throttling: { repeatInterval: 1h }
```

### AlertPolicy

```yaml
kind: AlertPolicy
metadata: { name: disk-health }
spec:
  enabled: true
  severity: critical
  conditions:
    - metric: novanas_disk_smart_failing
      operator: eq
      value: 1
      for: 5m
    - metric: novanas_disk_reallocated_sectors
      operator: gt
      value: 10
  channels: [admin-email, admin-ntfy]
  autoResolve: true
```

### Default alert rules shipped

Every appliance ships these pre-enabled (user can disable or customize):

- Disk SMART failing
- Pool degraded / rebuild slow / rebuild failed
- Scrub failed / scrub found errors
- Snapshot schedule missed
- Replication job failed / behind schedule
- CloudBackup job failed
- Node high CPU / memory / temperature
- Disk full > 90% / 95%
- Certificate expiring < 14 days
- OpenBao sealed / unhealthy
- Keycloak unhealthy
- Storage engine unhealthy
- Update available (informational)
- Security events (repeated failed login, 2FA brute force, admin actions)

## SLOs

```yaml
kind: ServiceLevelObjective
metadata: { name: s3-availability }
spec:
  target: { kind: ObjectStore, name: main }
  objective: 99.9
  window: 30d
  sli:
    type: availability                # availability | latency | error-budget
    metric: novanas_s3_request_success_ratio
```

Prometheus rules auto-generated → burn-rate alerts fire when error budget depletes too fast. Hidden behind "Advanced" in UI for admins who want them.

## Health API

Separate from raw metrics — hierarchical summary:

```
System ── healthy
  ├── Storage ── degraded (1 disk rebuilding)
  │    ├── Pool main ── degraded
  │    └── Pool cold ── healthy
  ├── Network ── healthy
  ├── Apps (23 running) ── healthy
  ├── VMs (3 running) ── warning (1 restart loop)
  ├── Updates ── update available
  └── Security ── healthy
```

UI top-bar badge summarizes to worst-child. Green/yellow/red; click to drill in.

## Diagnostic bundle

Admin UI button: "Generate support bundle" → NovaNas API creates encrypted tarball:

- Last N hours of structured logs (all components)
- CRD dump (secrets redacted)
- Prometheus metrics snapshot
- `kubectl describe` for all pods
- `nmstate show`, `ip`, `dmesg`, SMART outputs
- NovaStor chunk engine diagnostics (scrub results, recent I/O errors)
- System journal (journald)
- k3s state

**Encryption**: bundle is encrypted against a published NovaNas support public key so it can be shared over email/chat without leaking secrets. Admin can also decrypt locally with a passphrase.

## What we don't ship

- Custom log aggregation beyond Loki — no reinvention
- Flame graphs / profiling UI — `perf` and `pprof` available via admin shell
- APM-style per-user request tracking — Loki + Tempo cover this
