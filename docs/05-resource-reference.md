# 05 — Resource Reference

This is the catalog of business objects NovaNas exposes through the API
server.

**Every resource here is an API-server-owned object backed by Postgres,
reachable through `/api/v1/*`.** None of them are Kubernetes CRDs.
Internally, runtime-neutral controllers read these resources and emit
runtime-native objects (Pods/Services on Kubernetes today, container
primitives on Docker tomorrow) through the runtime adapter — see
[ADR 0005](adr/0005-hide-kubernetes-behind-api.md) and
[01-architecture-overview.md](01-architecture-overview.md).

The schemas below are the API request/response shapes (Zod-validated),
not Kubernetes manifests. There is no `apiVersion`/`kind` envelope and
no `kubectl get` for any of these.

## Resource map

### Storage primitives

| Resource | Purpose |
|---|---|
| `pool` | Bag of disks, tier label |
| `blockVolume` | Raw block device |
| `dataset` | BlockVolume + filesystem + mountable storage area |
| `bucket` | Native S3 object bucket (volume-equivalent) |
| `disk` | One per physical device; lifecycle state |

### Sharing

| Resource | Purpose |
|---|---|
| `share` | Multi-protocol export (SMB + NFS) of a Dataset path |
| `smbServer` | Samba server config |
| `nfsServer` | knfsd config |
| `iscsiTarget` | iSCSI portal binding a BlockVolume |
| `nvmeofTarget` | NVMe-oF subsystem binding a BlockVolume |
| `objectStore` | S3 gateway service config (port, TLS, features) |
| `bucketUser` | S3 credentials + policies |

### Identity

| Resource | Purpose |
|---|---|
| `user` | Local user (projection of Keycloak user) |
| `group` | Group (projection of Keycloak group) |
| `keycloakRealm` | Realm federation config (AD/LDAP/OIDC) |
| `apiToken` | Scoped API token |
| `sshKey` | SSH authorized keys for admin escape-hatch access |

### Data protection

| Resource | Purpose |
|---|---|
| `snapshot` | Point-in-time of BlockVolume / Dataset / Bucket |
| `snapshotSchedule` | Periodic Snapshot creator + retention |
| `replicationTarget` | Remote NovaNas endpoint |
| `replicationJob` | Snapshot-diff replication to a Target |
| `cloudBackupTarget` | S3/B2/Azure endpoint |
| `cloudBackupJob` | Volume → CloudBackupTarget (restic/borg/kopia) |
| `scrubSchedule` | Per-pool integrity scrub cadence |

### Networking

| Resource | Purpose |
|---|---|
| `physicalInterface` | Observed NIC (status-only) |
| `bond` | LACP / active-backup / balance |
| `vlan` | 802.1Q virtual interface |
| `hostInterface` | IP-bearing interface with role (management/storage/cluster/vmBridge) |
| `clusterNetwork` | Pod/service CIDRs, overlay config (consumed by the runtime adapter) |
| `vipPool` | novaedge LAN VIP range |
| `ingress` | novaedge reverse-proxy ingress rules |
| `remoteAccessTunnel` | SD-WAN / Tailscale-style tunnel |
| `customDomain` | User-supplied hostname pointing at an app |
| `firewallRule` | Host-level nftables or pod-level novanet policy |
| `trafficPolicy` | QoS limits by scope (interface/namespace/app/vm/job) |

### Apps & VMs

| Resource | Purpose |
|---|---|
| `appCatalog` | Catalog source (git/helm/oci), trust config |
| `app` | Synthesized catalog entry (read-only) |
| `appInstance` | User-installed app |
| `vm` | Virtual machine (NAS-friendly UX over the runtime's VM facility) |
| `isoLibrary` | Managed ISO collection for VM install |
| `gpuDevice` | Observed GPU, passthrough assignment |

### Encryption / KMS

| Resource | Purpose |
|---|---|
| `encryptionPolicy` | Cluster defaults for volume encryption |
| `kmsKey` | Named DK for SSE-KMS usage |
| `certificate` | TLS cert (ACME via novaedge / internal PKI / upload) |

### Ops & monitoring

| Resource | Purpose |
|---|---|
| `smartPolicy` | Disk SMART test cadence + thresholds |
| `alertChannel` | Email / webhook / ntfy / pushover / etc. |
| `alertPolicy` | Metric threshold → channel |
| `auditPolicy` | What to audit, where to send it |
| `serviceLevelObjective` | SLO config, auto-generates Prom rules |
| `upsPolicy` | NUT/apcupsd integration |

### System

| Resource | Purpose |
|---|---|
| `systemSettings` | Hostname, timezone, NTP, locale, SMTP |
| `updatePolicy` | Channel, auto-update, maintenance window |
| `configBackupPolicy` | Config snapshot cron + destinations |
| `servicePolicy` | Master enable/disable for SSH, SMB, NFS, etc. |

## Schema examples

The shapes below are the API request bodies. Create a resource with
`POST /api/v1/<collection>` (e.g. `POST /api/v1/pools`); update with
`PATCH /api/v1/<collection>/{name}`; list with `GET /api/v1/<collection>`.
WebSocket subscriptions on `/api/v1/<collection>/_stream` deliver
status updates (no `kubectl get -w`, no CRD watch).

### pool

```json
{
  "name": "main",
  "tier": "warm",
  "deviceFilter": {
    "preferredClass": "hdd",
    "minSize": "500Gi"
  },
  "recoveryRate": "balanced",
  "rebalanceOnAdd": "manual"
}
```

### dataset

```json
{
  "name": "family-media",
  "pool": "main",
  "size": "4Ti",
  "filesystem": "xfs",
  "protection": {
    "mode": "erasureCoding",
    "erasureCoding": { "dataShards": 4, "parityShards": 2 }
  },
  "aclMode": "nfsv4",
  "tiering": {
    "primary": "main",
    "demoteTo": "cold",
    "demoteAfter": "30d"
  },
  "encryption": { "enabled": true },
  "compression": "zstd",
  "quota": { "hard": "4Ti", "soft": "3.5Ti" },
  "defaults": { "owner": "pascal", "group": "family", "mode": "0770" }
}
```

### bucket

```json
{
  "name": "family-photos-archive",
  "store": "main",
  "protection": {
    "mode": "erasureCoding",
    "erasureCoding": { "dataShards": 4, "parityShards": 2 }
  },
  "tiering": { "primary": "fast", "demoteTo": "cold", "demoteAfter": "30d" },
  "encryption": { "enabled": true },
  "versioning": "enabled",
  "objectLock": {
    "enabled": true,
    "mode": "governance",
    "defaultRetention": { "period": "30d" }
  },
  "quota": { "hardBytes": "1Ti", "hardObjects": 10000000 },
  "lifecycle": [
    { "prefix": "temp/", "expireAfter": "7d" }
  ]
}
```

### share

```json
{
  "name": "photos",
  "dataset": "family-media",
  "path": "/photos",
  "protocols": {
    "smb": { "server": "main-smb", "shadowCopies": true, "caseSensitive": false },
    "nfs": { "server": "main-nfs", "squash": "rootSquash", "allowedNetworks": ["192.168.1.0/24"] }
  },
  "access": [
    { "principal": { "user": "pascal" }, "mode": "rw" },
    { "principal": { "group": "family" }, "mode": "rw" }
  ]
}
```

### disk

`disk` records are mostly system-observed; only `pool` and `role` are
user-mutable. Status is reported by the disk controller.

```json
{
  "name": "wwn-0x5000c500abc12345",
  "spec": { "pool": "main", "role": "data" },
  "status": {
    "slot": "enclosure-0/slot-3",
    "model": "WDC WD80EFAX-68L",
    "serial": "VHKX12Z9",
    "sizeBytes": 8001563222016,
    "smart": { "overallHealth": "OK", "temperature": 38, "powerOnHours": 12456 },
    "state": "ACTIVE"
  }
}
```

### snapshotSchedule

```json
{
  "name": "hourly-family",
  "source": { "kind": "dataset", "name": "family-media" },
  "cron": "0 * * * *",
  "retention": { "hourly": 24, "daily": 7, "weekly": 4, "monthly": 12 },
  "namingFormat": "@GMT-%Y.%m.%d-%H.%M.%S"
}
```

### replicationTarget + replicationJob

```json
{
  "name": "offsite",
  "endpoint": "https://nas.offsite.example.com",
  "auth": { "secretRef": "openbao://novanas/replication/offsite-token" },
  "transport": {
    "compression": "zstd",
    "encryption": true,
    "bandwidth": { "limit": "100Mbps", "schedule": "off-hours" }
  }
}
```

```json
{
  "name": "family-media-to-offsite",
  "source": { "kind": "dataset", "name": "family-media" },
  "target": "offsite",
  "direction": "push",
  "cron": "0 2 * * *",
  "retention": { "keepLast": 30 }
}
```

### appInstance

```json
{
  "name": "family-plex",
  "owner": "pascal",
  "app": "plex",
  "version": "1.40.3.8555",
  "values": { "mediaLibrary": "/data/media" },
  "storage": [
    { "name": "config", "dataset": "pascal/plex-config", "size": "5Gi" },
    { "name": "media", "dataset": "family-media", "mode": "ReadOnly" }
  ],
  "network": {
    "expose": [
      { "port": 32400, "protocol": "TCP", "advertise": "mdns", "tls": { "certificate": "plex-cert" } }
    ]
  },
  "updates": { "autoUpdate": false }
}
```

### vm

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
    }
  ],
  "cdrom": [
    { "name": "installer", "source": { "type": "iso", "isoLibrary": "family/win11.iso" } }
  ],
  "network": [{ "type": "bridge", "bridge": "br0" }],
  "gpu": { "passthrough": [{ "vendor": "nvidia", "device": "10de:2684" }] },
  "graphics": { "enabled": true, "type": "spice" },
  "autostart": "onBoot",
  "powerState": "Running"
}
```

### hostInterface / bond / vlan

```json
{ "name": "bond0", "interfaces": ["enp4s0", "enp5s0"], "mode": "802.3ad", "lacp": { "rate": "fast" }, "mtu": 9000 }
```

```json
{ "name": "storage-vlan", "parent": "bond0", "vlanId": 42, "mtu": 9000 }
```

```json
{
  "name": "storage",
  "backing": "storage-vlan",
  "addresses": [{ "cidr": "10.10.42.10/24", "type": "static" }],
  "mtu": 9000,
  "usage": ["storage"]
}
```

## API versioning

- `/api/v1alpha1/*` during pre-release iteration
- `/api/v1beta1/*` after core model stable and real appliances shipping
- `/api/v1/*` on GA
- The API server maintains backwards compatibility for one minor
  version after a promotion; no Kubernetes-style conversion webhooks
  are involved.

## How runtime objects are produced

Internally, NovaNas controllers translate these API resources into
runtime objects:

| API resource | Runtime objects (K8s adapter) | Runtime objects (Docker adapter, planned) |
|---|---|---|
| `share` (SMB protocol) | Pod (Samba), Service, NetworkPolicy, ConfigMap | container, port publish, network attach, mounted config |
| `vm` | KubeVirt VirtualMachine, DataVolume, Service | qemu-system invocation, libvirt domain, bridge attach |
| `appInstance` | Deployment/StatefulSet, Service, PVCs, ConfigMap | container(s), volumes, port publish |
| `iscsiTarget` | DaemonSet running tgtd config, Service | host-mode container running tgtd |
| `objectStore` | Deployment + Service for the S3 gateway | container with port publish |
| `bond` / `vlan` / `hostInterface` | nmstate config applied via host-agent | nmstate config applied via host-agent (runtime-agnostic) |

These translations are an internal contract owned by the runtime
adapter — they are **not part of the public NovaNas API** and may
change without an API version bump.
