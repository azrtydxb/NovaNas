# 03 — Access Protocols

Four protocol families, all backed by the same chunk engine:

- **Block**: iSCSI, NVMe-oF
- **File**: NFS, SMB
- **Object**: S3

## Unifying pattern for block and file

```
Chunk Engine ──► Block Device ──► (FS format) ──► Protocol pod ──► Clients
                             └──► iSCSI/NVMe-oF target ──► Block clients
```

Every file and block protocol exposes a block device from the chunk engine. File protocols put a filesystem on top and run a protocol server pod. Block protocols expose the device directly.

Object storage does not follow this pattern — see the S3 section.

## Block protocols

### iSCSI

- `IscsiTarget` CRD binds a BlockVolume to an iSCSI portal
- Target implementation: kernel LIO (tcm-loop) configured via targetcli from an operator
- CHAP auth support, mutual CHAP optional
- Multiple initiators via `aclMode: any | whitelist`

### NVMe-oF

- `NvmeofTarget` CRD binds a BlockVolume to an NVMe subsystem
- Target: SPDK nvmf_tgt (already part of NovaStor dataplane)
- TCP transport (RDMA possible future)
- Host NQN whitelist

Both block protocols are exposed on HostInterfaces whose `usage` includes `storage`.

## File protocols

### NFS (knfsd via operator pod)

- NFS runs as kernel NFS (knfsd) on the host — same approach as TrueNAS SCALE
- `novanas-nfs-operator` pod (privileged, hostPath) writes `/etc/exports` and runs `exportfs -ra`
- Supports NFSv3 and NFSv4.1+
- Export configuration comes from `Share` CRDs with `protocols.nfs`

Why host knfsd instead of Ganesha:
- Significantly faster, lower CPU
- Universal NAS-appliance choice (TrueNAS, Synology, QNAP, Unraid)
- Trade-off: operator pod must be privileged and write host `/etc/exports`

### SMB (Samba in a pod)

- Upstream Samba in a userspace pod in `novanas-system`
- Mounts BlockVolume(s) via CSI, filesystem (xfs/ext4) visible as `/mnt/share/<name>`
- Shares exported from subdirectories of mounted Datasets
- Standard kernel filesystem handles ACLs in xattr (NTFS ACL emulation), case sensitivity, oplocks/leases
- `vfs_shadow_copy2` surfaces NovaNas snapshots as Windows "Previous Versions"
- AD join via `net ads join` (Samba's native support); winbind sidecar for AD SID ↔ POSIX UID mapping
- LDAP via sssd in the pod for non-AD directory setups

### Multi-protocol Share

A single `Share` CRD exposes one path over both SMB and NFS simultaneously. The Share operator writes Samba config + `/etc/exports` in sync.

```yaml
kind: Share
spec:
  dataset: family-media
  path: /photos
  protocols:
    smb: { server: main-smb, shadowCopies: true }
    nfs: { server: main-nfs, allowedNetworks: [192.168.1.0/24] }
  access:
    - principal: { user: pascal }
      mode: rw
    - principal: { group: family }
      mode: rw
```

### ACL model

Two modes per Dataset, set at creation:

- **`posix`** — traditional u/g/o + POSIX ACLs. Simple, Linux-native, `ls -l` is truthful. Samba synthesizes best-effort NTFS ACLs for Windows clients; complex inheritance and deny rules don't round-trip cleanly.
- **`nfsv4`** — rich ACLs stored in xattr. Full NTFS-equivalent semantics (allow/deny, inheritance, granular rights). Lossless Windows round-trip. `ls -l` shows synthesized mode bits that may not tell the full truth.

Switching modes on a populated dataset is destructive. Locked at creation.

UI default: auto-picks `nfsv4` when SMB shares exist, `posix` when NFS/Linux-only. Power users can override in an Advanced panel.

## Object protocol (S3)

Native implementation — NOT block-device-backed, NOT MinIO.

### Why native

- Buckets are first-class volumes, peers of BlockVolume and Dataset
- Chunks are content-addressed and immutable; objects are natively chunk-shaped
- Free dedup across objects + files + blocks when the same bytes are stored multiple times
- License control (no MinIO AGPL/commercial-terms risk)
- Full chunk-engine integration with encryption, snapshots, replication

### Architecture

```
┌────────────────────────────────────┐
│ novanas-s3gw (Deployment)          │
│  - S3 HTTP/HTTPS API               │
│  - SigV4 auth, presigned URLs       │
│  - Multipart state machine         │
│  - Versioning, Object Lock, ACLs   │
│  - Bucket lifecycle policies       │
│  - S3 event notifications          │
│  - S3 website hosting              │
│  - Object Select (SQL over CSV/    │
│    JSON/Parquet)                   │
│  - S3 Replication Rules            │
└─────────────┬──────────────────────┘
              │ gRPC
┌─────────────▼──────────────────────┐
│ Metadata + Chunk Engine            │
└────────────────────────────────────┘
```

### Compatibility scope (v1)

Full AWS S3 surface — all CRUD, multipart, presigned URLs, versioning, ACLs, Object Lock, SSE-S3/SSE-C/SSE-KMS, Object Select, event notifications, Glacier-API semantics, S3 website hosting, Replication Rules.

### Object Lock

- Enabled only at bucket creation (AWS-compatible, immutable after)
- When enabled, `mode: governance | compliance` must be chosen explicitly
- `governance` can be bypassed by users with `bypassGovernance: true` policy flag (audited)
- `compliance` cannot be bypassed — only escape is factory reset
- Enforcement ripples into:
  - Snapshot retention (locked snapshots cannot be deleted until retention expires)
  - Chunk GC (locked chunks cannot be collected)
  - Disk removal (must migrate, not destroy)
  - Pool deletion (blocked while locks exist)
  - Replication targets must honor the lock on the remote side

### Server-side encryption

Three variants translated to internal convergent encryption:

| SSE variant | Internal mapping | Dedup |
|---|---|---|
| `AES256` (SSE-S3) | Bucket DK (automatic) | Yes, within bucket |
| `aws:kms` (SSE-KMS) | `KmsKey` CRD → named DK | Yes, within KmsKey scope |
| customer-provided (SSE-C) | Per-object client key, segregated chunk namespace | No (by design) |

### CI quality bar

Merges blocked unless all pass:
- MinIO `mint` test suite
- Ceph `s3-tests` test suite
- AWS SDK smoke tests (boto3, go-sdk, aws-sdk-js)

## Quality-of-service

All protocol traffic can be shaped via `TrafficPolicy` CRD — interface-level, per-namespace, per-app, per-VM, per-replication-job, per-ObjectStore.

## Default ports

| Protocol | Port |
|---|---|
| HTTPS UI/API | 443 |
| SSH (if enabled) | 22 |
| SMB | 445 |
| NFS | 2049 |
| iSCSI | 3260 |
| NVMe-oF TCP | 4420 |
| S3 | 9000 (configurable) |

No default host firewall. Ports are open per `ServicePolicy`.
