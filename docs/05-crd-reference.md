# 05 — CRD Reference

All CRDs are in the API group `novanas.io/v1alpha1`, cluster-scoped unless noted. CRDs are the **internal** source of truth for operators; **users never see them** — they interact via the NovaNas API server.

## Resource map

### Storage primitives

| CRD | Short | Scope | Purpose |
|---|---|---|---|
| `StoragePool` | `sp` | cluster | Bag of disks, tier label |
| `BlockVolume` | `bv` | cluster | Raw block device |
| `Dataset` | `ds` | cluster | BlockVolume + filesystem + mountable storage area |
| `Bucket` | `bk` | cluster | Native S3 object bucket (volume-equivalent) |
| `Disk` | — | cluster | One per physical device; lifecycle state |

### Sharing

| CRD | Purpose |
|---|---|
| `Share` | Multi-protocol export (SMB + NFS) of a Dataset path |
| `SmbServer` | Samba pod deployment config |
| `NfsServer` | knfsd operator config |
| `IscsiTarget` | iSCSI portal binding a BlockVolume |
| `NvmeofTarget` | NVMe-oF subsystem binding a BlockVolume |
| `ObjectStore` | S3 gateway service config (port, TLS, features) |
| `BucketUser` | S3 credentials + policies |

### Identity

| CRD | Purpose |
|---|---|
| `User` | Local user (projection of Keycloak user) |
| `Group` | Group (projection of Keycloak group) |
| `KeycloakRealm` | Realm federation config (AD/LDAP/OIDC) |
| `ApiToken` | Scoped API token |
| `SshKey` | SSH authorized keys for admin escape-hatch access |

### Data protection

| CRD | Purpose |
|---|---|
| `Snapshot` | Point-in-time of BlockVolume / Dataset / Bucket |
| `SnapshotSchedule` | Periodic Snapshot creator + retention |
| `ReplicationTarget` | Remote NovaNas endpoint |
| `ReplicationJob` | Snapshot-diff replication to a Target |
| `CloudBackupTarget` | S3/B2/Azure endpoint |
| `CloudBackupJob` | Volume → CloudBackupTarget (restic/borg/kopia) |
| `ScrubSchedule` | Per-pool integrity scrub cadence |

### Networking

| CRD | Purpose |
|---|---|
| `PhysicalInterface` | Observed NIC (status-only) |
| `Bond` | LACP / active-backup / balance |
| `Vlan` | 802.1Q virtual interface |
| `HostInterface` | IP-bearing interface with role (management/storage/cluster/vmBridge) |
| `ClusterNetwork` | Pod/service CIDRs, overlay config |
| `VipPool` | novaedge LAN VIP range |
| `Ingress` | novaedge reverse-proxy ingress rules |
| `RemoteAccessTunnel` | SD-WAN / Tailscale-style tunnel |
| `CustomDomain` | User-supplied hostname pointing at an app |
| `FirewallRule` | Host-level nftables or pod-level novanet policy |
| `TrafficPolicy` | QoS limits by scope (interface/namespace/app/vm/job) |

### Apps & VMs

| CRD | Purpose |
|---|---|
| `AppCatalog` | Catalog source (git/helm/oci), trust config |
| `App` | Synthesized catalog entry (read-only) |
| `AppInstance` | User-installed app |
| `Vm` | KubeVirt VM with NAS-friendly UX |
| `IsoLibrary` | Managed ISO collection for VM install |
| `GpuDevice` | Observed GPU, passthrough assignment |

### Encryption / KMS

| CRD | Purpose |
|---|---|
| `EncryptionPolicy` | Cluster defaults for volume encryption |
| `KmsKey` | Named DK for SSE-KMS usage |
| `Certificate` | TLS cert (ACME via novaedge / internal PKI / upload) |

### Ops & monitoring

| CRD | Purpose |
|---|---|
| `SmartPolicy` | Disk SMART test cadence + thresholds |
| `AlertChannel` | Email / webhook / ntfy / pushover / etc. |
| `AlertPolicy` | Metric threshold → channel |
| `AuditPolicy` | What to audit, where to send it |
| `ServiceLevelObjective` | SLO config, auto-generates Prom rules |
| `UpsPolicy` | NUT/apcupsd integration |

### System

| CRD | Purpose |
|---|---|
| `SystemSettings` | Hostname, timezone, NTP, locale, SMTP |
| `UpdatePolicy` | Channel, auto-update, maintenance window |
| `ConfigBackupPolicy` | Config snapshot cron + destinations |
| `ServicePolicy` | Master enable/disable for SSH, SMB, NFS, etc. |

## Example manifests

### StoragePool

```yaml
apiVersion: novanas.io/v1alpha1
kind: StoragePool
metadata:
  name: main
spec:
  tier: warm
  deviceFilter:
    preferredClass: hdd
    minSize: 500Gi
  recoveryRate: balanced
  rebalanceOnAdd: manual
```

### Dataset

```yaml
apiVersion: novanas.io/v1alpha1
kind: Dataset
metadata:
  name: family-media
spec:
  pool: main
  size: 4Ti
  filesystem: xfs
  protection:
    mode: erasureCoding
    erasureCoding: { dataShards: 4, parityShards: 2 }
  aclMode: nfsv4
  tiering:
    primary: main
    demoteTo: cold
    demoteAfter: 30d
  encryption:
    enabled: true
  compression: zstd
  quota:
    hard: 4Ti
    soft: 3.5Ti
  defaults:
    owner: pascal
    group: family
    mode: "0770"
```

### Bucket

```yaml
apiVersion: novanas.io/v1alpha1
kind: Bucket
metadata:
  name: family-photos-archive
spec:
  store: main
  protection:
    mode: erasureCoding
    erasureCoding: { dataShards: 4, parityShards: 2 }
  tiering:
    primary: fast
    demoteTo: cold
    demoteAfter: 30d
  encryption:
    enabled: true
  versioning: enabled
  objectLock:
    enabled: true
    mode: governance
    defaultRetention: { period: 30d }
  quota: { hardBytes: 1Ti, hardObjects: 10000000 }
  lifecycle:
    - prefix: temp/
      expireAfter: 7d
```

### Share

```yaml
apiVersion: novanas.io/v1alpha1
kind: Share
metadata:
  name: photos
spec:
  dataset: family-media
  path: /photos
  protocols:
    smb:
      server: main-smb
      shadowCopies: true
      caseSensitive: false
    nfs:
      server: main-nfs
      squash: rootSquash
      allowedNetworks: [192.168.1.0/24]
  access:
    - principal: { user: pascal }
      mode: rw
    - principal: { group: family }
      mode: rw
```

### Disk

```yaml
apiVersion: novanas.io/v1alpha1
kind: Disk
metadata:
  name: wwn-0x5000c500abc12345
spec:
  pool: main
  role: data
status:
  slot: enclosure-0/slot-3
  model: "WDC WD80EFAX-68L"
  serial: "VHKX12Z9"
  sizeBytes: 8001563222016
  smart:
    overallHealth: OK
    temperature: 38
    powerOnHours: 12456
  state: ACTIVE
```

### Snapshot + Schedule

```yaml
apiVersion: novanas.io/v1alpha1
kind: SnapshotSchedule
metadata:
  name: hourly-family
spec:
  source: { kind: Dataset, name: family-media }
  cron: "0 * * * *"
  retention:
    hourly: 24
    daily: 7
    weekly: 4
    monthly: 12
  namingFormat: "@GMT-%Y.%m.%d-%H.%M.%S"
```

### ReplicationTarget + Job

```yaml
apiVersion: novanas.io/v1alpha1
kind: ReplicationTarget
metadata:
  name: offsite
spec:
  endpoint: https://nas.offsite.example.com
  auth:
    secretRef: openbao://novanas/replication/offsite-token
  transport:
    compression: zstd
    encryption: true
    bandwidth: { limit: 100Mbps, schedule: "off-hours" }
---
apiVersion: novanas.io/v1alpha1
kind: ReplicationJob
metadata:
  name: family-media-to-offsite
spec:
  source: { kind: Dataset, name: family-media }
  target: offsite
  direction: push
  cron: "0 2 * * *"
  retention: { keepLast: 30 }
```

### AppInstance

```yaml
apiVersion: novanas.io/v1alpha1
kind: AppInstance
metadata:
  name: family-plex
  namespace: novanas-users/pascal
spec:
  app: plex
  version: 1.40.3.8555
  values:
    mediaLibrary: /data/media
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
        advertise: mdns
        tls: { certificate: plex-cert }
  updates:
    autoUpdate: false
```

### Vm

```yaml
apiVersion: novanas.io/v1alpha1
kind: Vm
metadata:
  name: windows-11
  namespace: novanas-vms
spec:
  owner: pascal
  os: { type: windows, variant: win11 }
  resources: { cpu: 4, memoryMiB: 8192 }
  disks:
    - name: system
      source: { type: dataset, dataset: pascal/win11-system, size: 80Gi }
      bus: virtio
      boot: 1
  cdrom:
    - name: installer
      source: { type: iso, isoLibrary: family/win11.iso }
  network:
    - type: bridge
      bridge: br0
  gpu:
    passthrough:
      - vendor: nvidia
        device: "10de:2684"
  graphics: { enabled: true, type: spice }
  autostart: onBoot
  powerState: Running
```

### HostInterface / Bond / Vlan

```yaml
apiVersion: novanas.io/v1alpha1
kind: Bond
metadata: { name: bond0 }
spec:
  interfaces: [enp4s0, enp5s0]
  mode: 802.3ad
  lacp: { rate: fast }
  mtu: 9000
---
apiVersion: novanas.io/v1alpha1
kind: Vlan
metadata: { name: storage-vlan }
spec: { parent: bond0, vlanId: 42, mtu: 9000 }
---
apiVersion: novanas.io/v1alpha1
kind: HostInterface
metadata: { name: storage }
spec:
  backing: storage-vlan
  addresses:
    - { cidr: 10.10.42.10/24, type: static }
  mtu: 9000
  usage: [storage]
```

## API versioning

- `v1alpha1` during pre-release iteration
- `v1beta1` after core model stable and real appliances shipping
- `v1` on GA
- Conversion webhooks run during upgrades; previous version supported for one release cycle after promotion
