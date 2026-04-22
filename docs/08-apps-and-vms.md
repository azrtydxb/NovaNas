# 08 — Apps & VMs

NovaNas is both a storage appliance and a home-server / small-business host. Users deploy containers (apps) and virtual machines (VMs) alongside their storage.

## Apps

An App is a user-facing wrapper around a Helm chart rendered into the user's namespace.

### Three tiers

| Tier | Source | Trust |
|---|---|---|
| **Official** | NovaNas team curated | cosign-signed, catalog-verified, tested in CI |
| **Community** | Third-party catalogs (TrueCharts-style) | Registered with NovaNas, not signed by NovaNas |
| **Custom** | User-uploaded Helm chart | User's own responsibility |

`AppCatalog` CRDs register sources. Catalog entries synthesize `App` CRs (read-only catalog reflections). Users instantiate `AppInstance` from an `App` in their own namespace.

### AppCatalog

```yaml
apiVersion: novanas.io/v1alpha1
kind: AppCatalog
metadata: { name: official }
spec:
  source:
    type: git
    url: https://github.com/azrtydxb/novanas-apps
    branch: main
    refreshInterval: 1h
  trust:
    signedBy: novanas-official-keyring
    required: true
```

### App (synthesized)

```yaml
kind: App
metadata: { name: plex, labels: { catalog: official, category: media } }
spec:
  displayName: Plex Media Server
  version: 1.40.3.8555
  icon: https://.../plex.png
  description: "..."
  schema: {...}                # JSON schema for user-tunable values
  chart: { ociRef: ghcr.io/azrtydxb/charts/plex:1.40.3.8555, digest: sha256:... }
  requirements:
    minRamMB: 2048
    requiresGpu: false
    ports: [32400]
```

### AppInstance

```yaml
kind: AppInstance
metadata: { name: family-plex, namespace: novanas-users/pascal }
spec:
  app: plex
  version: 1.40.3.8555
  values:
    mediaLibrary: /data/media
    adminEmail: pascal@watteel.com
  storage:
    - name: config
      dataset: pascal/plex-config
      size: 5Gi
    - name: media
      dataset: family-media
      mode: ReadOnly
  network:
    expose:
      - port: 32400
        protocol: TCP
        advertise: mdns            # mdns | lan | reverseProxy | internet
        tls: { certificate: plex-cert }
  updates:
    autoUpdate: false
status:
  phase: Running
  healthy: true
  revision: 3
  exposedAt: https://plex.nas.local
```

### Custom Helm charts

Advanced users can upload arbitrary Helm charts as `AppCatalog` of type `custom`. Charts are rendered into the user's namespace with PSA `restricted`. If the chart requires privileges beyond `restricted`, installation fails — user must work with an admin to install into `novanas-apps-system`.

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

Apps are first-class sources for existing CRDs:

- `SnapshotSchedule` source: `kind: AppInstance` → snapshots all datasets/volumes the app uses + the AppInstance spec itself
- `ReplicationJob` source: `kind: AppInstance` → replicates storage + config bundle
- `CloudBackupJob` source: `kind: AppInstance` → offsite backup

## Virtual Machines (KubeVirt)

KubeVirt runs in `novanas-system`. VMs live in `novanas-vms` namespace with owner labels.

### Vm CRD

```yaml
kind: Vm
metadata: { name: windows-11, namespace: novanas-vms }
spec:
  owner: pascal
  os: { type: windows, variant: win11 }
  resources: { cpu: 4, memoryMiB: 8192 }
  disks:
    - name: system
      source: { type: dataset, dataset: pascal/win11-system, size: 80Gi }
      bus: virtio
      boot: 1
    - name: data
      source: { type: blockVolume, blockVolume: pascal/win11-data }
      bus: virtio
  cdrom:
    - name: installer
      source: { type: iso, isoLibrary: family/win11.iso }
  network:
    - type: bridge
      bridge: br0
      mac: auto
  gpu:
    passthrough:
      - vendor: nvidia
        device: "10de:2684"
  graphics: { enabled: true, type: spice }
  autostart: onBoot
  powerState: Running
status:
  phase: Running
  consoleUrl: wss://nas.local/vms/windows-11/console
  ip: 192.168.1.101
```

### Disk sources

- `dataset` — Dataset with xfs/ext4 containing a qcow2 (easy snapshots, growable)
- `blockVolume` — raw BlockVolume (best performance, no FS overhead)
- `iso` — read-only ISO from `IsoLibrary`
- `clone` — instantaneous clone of another VM's disk (chunk-dedup makes this free)

### ISO library

```yaml
kind: IsoLibrary
spec:
  dataset: isos                    # Dataset holding all ISO files
  sources:
    - url: https://.../ubuntu-24.04.iso
      sha256: "..."
```

Operator downloads, verifies, and makes available as VM-attachable ISOs.

### GPU passthrough

- `novanas-gpu-manager` DaemonSet enumerates GPUs, surfaces `GpuDevice` CRs
- Requires IOMMU enabled in BIOS/UEFI
- `vfio-pci` kernel binding configured at boot via kernel command-line and systemd
- Admin assigns specific `GpuDevice` to VMs; cannot be shared
- Once assigned to VFIO, the GPU is unavailable for host (no desktop on that GPU)

```yaml
kind: GpuDevice
metadata: { name: gpu-0 }
spec:
  passthrough: true
status:
  vendor: NVIDIA
  model: RTX 4080
  pciAddress: "0000:03:00.0"
  assignedTo: { kind: Vm, name: windows-11 }
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
- Backup/replication using same CRDs as apps and datasets

## Storage unification

This is where NovaNas shines: apps, VMs, datasets, buckets all share the chunk engine:

- Deduplication across all of it (VM disk image + file share + S3 object with same bytes = one copy)
- One `ReplicationJob` can replicate a whole "solution" (app + data + VM) as a coordinated snapshot bundle
- One `CloudBackupJob` can back up anything off-box
- Snapshot-before-upgrade works uniformly for apps and VMs

## Quotas and resource limits

Per-user `ResourceQuota` enforces:
- CPU/memory across all apps and VMs in the user's namespace
- Storage requests (PVC) against user's storage quota
- Object count limits (pod, PVC)

`LimitRange` enforces sensible per-pod defaults and caps.

## Apps that need system-level privilege

Some apps (e.g., Pi-hole wanting to bind port 53, AdGuard Home as DNS) need more than `restricted`:

- Must be Official-catalog
- Install target: `novanas-apps-system` (PSA `baseline`)
- Chart manifests reviewed in official catalog PR process
- User visibility: optional — can be exposed to users as "System Services" view

## App/VM observability

- Per-app pod metrics (CPU, memory, IO, restart count) auto-scraped
- Per-VM metrics via KubeVirt exporter
- User sees their own apps/VMs in their dashboard; admin sees all
- Logs via Loki, searchable in UI
