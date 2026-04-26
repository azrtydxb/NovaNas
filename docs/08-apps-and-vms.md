# 08 — Apps & VMs

NovaNas is both a storage appliance and a home-server / small-business host. Users deploy containers (apps) and virtual machines (VMs) alongside their storage.

## Apps

An App is a user-facing wrapper around a packaged container workload. NovaNas uses Helm charts as a *packaging format* for the app catalog (versioning, templating, signing), but the rendered output is consumed by the app controller, which then asks the runtime adapter to materialize the workload (Pods/Deployments/Services on K8s today, containers/networks/volumes on Docker tomorrow). Helm is **not** the runtime authority — the API server is.

### Three tiers

| Tier | Source | Trust |
|---|---|---|
| **Official** | NovaNas team curated | cosign-signed, catalog-verified, tested in CI |
| **Community** | Third-party catalogs (TrueCharts-style) | Registered with NovaNas, not signed by NovaNas |
| **Custom** | User-uploaded Helm chart | User's own responsibility |

`appCatalog` API resources register sources. Catalog entries are synthesized into read-only `app` API resources (catalog reflections). Users create an `appInstance` from an `app` in their own tenant.

### appCatalog

`POST /api/v1/appCatalogs`:

```json
{
  "name": "official",
  "source": {
    "type": "git",
    "url": "https://github.com/azrtydxb/novanas-apps",
    "branch": "main",
    "refreshInterval": "1h"
  },
  "trust": {
    "signedBy": "novanas-official-keyring",
    "required": true
  }
}
```

### app (synthesized, read-only)

```json
{
  "name": "plex",
  "labels": { "catalog": "official", "category": "media" },
  "displayName": "Plex Media Server",
  "version": "1.40.3.8555",
  "icon": "https://.../plex.png",
  "description": "...",
  "schema": { "...": "JSON schema for user-tunable values" },
  "chart": {
    "ociRef": "ghcr.io/azrtydxb/charts/plex:1.40.3.8555",
    "digest": "sha256:..."
  },
  "requirements": { "minRamMB": 2048, "requiresGpu": false, "ports": [32400] }
}
```

### appInstance

`POST /api/v1/appInstances`:

```json
{
  "name": "family-plex",
  "owner": "pascal",
  "app": "plex",
  "version": "1.40.3.8555",
  "values": {
    "mediaLibrary": "/data/media",
    "adminEmail": "pascal@watteel.com"
  },
  "storage": [
    { "name": "config", "dataset": "pascal/plex-config", "size": "5Gi" },
    { "name": "media",  "dataset": "family-media", "mode": "ReadOnly" }
  ],
  "network": {
    "expose": [
      {
        "port": 32400,
        "protocol": "TCP",
        "advertise": "mdns",
        "tls": { "certificate": "plex-cert" }
      }
    ]
  },
  "updates": { "autoUpdate": false }
}
```

Status (read via `GET` or `_stream`):

```json
{
  "phase": "Running",
  "healthy": true,
  "revision": 3,
  "exposedAt": "https://plex.nas.local"
}
```

### Custom Helm charts

Advanced users can upload arbitrary Helm charts as a `custom` `appCatalog`. The app controller renders the chart and asks the runtime adapter to install it under restricted policy (PSA `restricted` on K8s; equivalent rootless/dropped-capabilities profile on Docker). If the chart requires privileges beyond the restricted profile, installation fails — user must work with an admin to install into the system tenant.

### Network exposure modes

All flow through novanet + novaedge:

| Mode | What happens |
|---|---|
| `mdns` | LAN VIP from `VipPool`, `<app>.nas.local` advertised via mDNS/SSDP/WS-Discovery |
| `lan` | LAN VIP, no advertisement (manually-known IP) |
| `reverseProxy` | novaedge reverse-proxy rule at `<app>.nas.local` with wildcard TLS — default for new apps |
| `internet` | novaedge SD-WAN tunnel or published VIP — admin-permission-gated |

### App updates

1. Catalog refresh detects new version
2. UI shows "Update available: 1.40.3 → 1.41.0"
3. User clicks Update → **pre-update snapshot** of all AppInstance storage → new values applied → Helm upgrade
4. Health check on rollout → failure triggers Helm rollback + snapshot restore
5. Pre-update snapshots kept **30 days** (configurable) for manual rollback

### Uninstall

- Default: remove pods / Services / Ingresses, **keep PVCs** (user data preserved)
- Explicit "Uninstall and delete all data" option for cleanup

### Initial official catalog (~30 apps at launch)

| Category | Apps |
|---|---|
| Media | Plex, Jellyfin, Emby |
| *arr | Sonarr, Radarr, Prowlarr, Lidarr, Readarr, Bazarr, qBittorrent |
| Photos | Immich, PhotoPrism |
| Files | Nextcloud, Seafile, Filebrowser |
| Home | Home Assistant, Frigate, Zigbee2MQTT |
| Dev | Gitea, Woodpecker, code-server |
| Databases | Postgres, MySQL, MariaDB, Redis, MongoDB |
| Observability (user-owned) | Prometheus, Grafana, Loki |
| Utility | Vaultwarden, Paperless-ngx, Bookstack, Wiki.js |
| Networking | AdGuard Home, Pi-hole, Nginx Proxy Manager |
| Backup | Duplicati, Restic UI, Kopia UI |

### Backup / replication integration

Apps are first-class sources for the data-protection API resources:

- `snapshotSchedule` with `source: { kind: appInstance, ... }` → snapshots all datasets/volumes the app uses plus the `appInstance` record itself
- `replicationJob` with `source: { kind: appInstance, ... }` → replicates storage + config bundle
- `cloudBackupJob` with `source: { kind: appInstance, ... }` → offsite backup

## Virtual Machines

The VM controller targets a runtime-supplied virtualization facility. On the Kubernetes adapter that's KubeVirt running in the system tenant; on a Docker adapter it would be libvirt/qemu invoked directly on the host. Either way the user-visible model is the same `vm` API resource.

### vm

`POST /api/v1/vms`:

```json
{
  "name": "windows-11",
  "owner": "pascal",
  "os": { "type": "windows", "variant": "win11" },
  "resources": { "cpu": 4, "memoryMiB": 8192 },
  "disks": [
    {
      "name": "system",
      "source": { "type": "dataset", "dataset": "pascal/win11-system", "size": "80Gi" },
      "bus": "virtio",
      "boot": 1
    },
    {
      "name": "data",
      "source": { "type": "blockVolume", "blockVolume": "pascal/win11-data" },
      "bus": "virtio"
    }
  ],
  "cdrom": [
    { "name": "installer", "source": { "type": "iso", "isoLibrary": "family/win11.iso" } }
  ],
  "network": [
    { "type": "bridge", "bridge": "br0", "mac": "auto" }
  ],
  "gpu": { "passthrough": [{ "vendor": "nvidia", "device": "10de:2684" }] },
  "graphics": { "enabled": true, "type": "spice" },
  "autostart": "onBoot",
  "powerState": "Running"
}
```

Status:

```json
{
  "phase": "Running",
  "consoleUrl": "wss://nas.local/vms/windows-11/console",
  "ip": "192.168.1.101"
}
```

### Disk sources

- `dataset` — `dataset` with xfs/ext4 containing a qcow2 (easy snapshots, growable)
- `blockVolume` — raw `blockVolume` (best performance, no FS overhead)
- `iso` — read-only ISO from an `isoLibrary`
- `clone` — instantaneous clone of another VM's disk (chunk-dedup makes this free)

### ISO library

`POST /api/v1/isoLibraries`:

```json
{
  "name": "family",
  "dataset": "isos",
  "sources": [
    { "url": "https://.../ubuntu-24.04.iso", "sha256": "..." }
  ]
}
```

The ISO library controller downloads, verifies, and makes the file available as a VM-attachable ISO.

### GPU passthrough

- A `novanas-gpu-agent` (host-mode container, one per node) enumerates GPUs and reports them via `POST/PATCH /api/v1/gpuDevices`
- Requires IOMMU enabled in BIOS/UEFI
- `vfio-pci` kernel binding configured at boot via kernel command-line and systemd
- Admin assigns a specific `gpuDevice` to a VM via the API; cannot be shared
- Once assigned to VFIO, the GPU is unavailable for host (no desktop on that GPU)

```json
{
  "name": "gpu-0",
  "spec": { "passthrough": true },
  "status": {
    "vendor": "NVIDIA",
    "model": "RTX 4080",
    "pciAddress": "0000:03:00.0",
    "assignedTo": { "kind": "vm", "name": "windows-11" }
  }
}
```

### Console

- **SPICE over WebSocket** via `spice-html5` in the NovaNas UI
- Full keyboard/mouse/clipboard passthrough
- Display sized to browser window, auto-resize
- Audio optional (SPICE agent required in guest)

### VM operations

- Start / Stop / Pause / Reset / Shutdown (ACPI)
- Live snapshot (memory + disk) — optional, slower
- Offline snapshot (disk only) — fast, chunk-level
- Clone (full instantaneous, backed by chunk dedup)
- Migrate (live migration) — N/A on single-node; schema ready for multi-node later
- Backup/replication using the same API resources as apps and datasets

## Storage unification

This is where NovaNas shines: apps, VMs, datasets, buckets all share the chunk engine:

- Deduplication across all of it (VM disk image + file share + S3 object with same bytes = one copy)
- One `replicationJob` can replicate a whole "solution" (app + data + VM) as a coordinated snapshot bundle
- One `cloudBackupJob` can back up anything off-box
- Snapshot-before-upgrade works uniformly for apps and VMs

## Quotas and resource limits

Quotas live on the user/tenant API resource and are enforced at two layers:

1. **API server admission**: storage byte/object quotas, app/VM count limits, and CPU/memory budgets are checked when an `appInstance` or `vm` is created or scaled.
2. **Runtime adapter enforcement**: the adapter applies runtime-native limits — `ResourceQuota` / `LimitRange` on Kubernetes, cgroup limits on Docker — so containers cannot exceed the budget the API approved.

The user-visible quota model is identical regardless of runtime; only the projection differs.

## Apps that need system-level privilege

Some apps (e.g., Pi-hole wanting to bind port 53, AdGuard Home as DNS) need more than the restricted profile:

- Must be Official-catalog
- Install target: the `novanas-apps-system` tenant (baseline profile — PSA `baseline` on K8s, equivalent host-port/cap allowances on Docker)
- Chart manifests reviewed in official catalog PR process
- User visibility: optional — can be exposed to users as "System Services" view

## App/VM observability

- Per-container metrics (CPU, memory, IO, restart count) auto-scraped — source depends on runtime adapter (cAdvisor on K8s, container metrics socket on Docker)
- Per-VM metrics via the runtime's VM exporter (KubeVirt exporter on K8s; libvirt-exporter on Docker)
- User sees their own apps/VMs in their dashboard; admin sees all
- Logs via Loki, searchable in UI
