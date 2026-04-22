# @novanas/schemas

Single source of truth for all NovaNas domain types.

Every CRD in the `novanas.io/v1alpha1` API group has a Zod schema and an inferred
TypeScript type. This package is consumed by the API server (validation), the UI
(forms), the CLI (types), and will be the basis for Go-type generation used by
operators in a later wave.

## Usage

```ts
import { DatasetSchema, type Dataset } from '@novanas/schemas';

const parsed: Dataset = DatasetSchema.parse({
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'Dataset',
  metadata: { name: 'family-media' },
  spec: {
    pool: 'main',
    size: '4Ti',
    filesystem: 'xfs',
    protection: {
      mode: 'erasureCoding',
      erasureCoding: { dataShards: 4, parityShards: 2 },
    },
  },
});
```

## Conventions

- Every top-level CRD exports `XSchema`, `XSpecSchema`, `XStatusSchema`, plus their
  `X`, `XSpec`, `XStatus` inferred types.
- `apiVersion` is a literal (`novanas.io/v1alpha1`), `kind` is a literal matching
  the CRD name.
- Status objects are declared with `.partial()` — all status fields are optional.
- Byte sizes are strings following Kubernetes quantity syntax (`BytesQuantity`).
- Durations are strings (`30d`, `1h`). Cron strings use the `CronSchema`.
- Volume source references (snapshots, replication, backup) are discriminated
  unions on `kind` (`VolumeSourceRef`).
- Protection policies are discriminated unions on `mode` (`replication`
  vs. `erasureCoding`).

## Layout

```
src/
├── index.ts                  re-exports every domain
├── common/                   primitives: quantities, metadata, enums, conditions, refs
├── storage/                  StoragePool, BlockVolume, Dataset, Bucket, Disk, ProtectionPolicy
├── sharing/                  Share, SmbServer, NfsServer, IscsiTarget, NvmeofTarget, ObjectStore, BucketUser
├── identity/                 User, Group, KeycloakRealm, ApiToken, SshKey
├── data-protection/          Snapshot, SnapshotSchedule, Replication*, CloudBackup*, ScrubSchedule
├── networking/               PhysicalInterface, Bond, Vlan, HostInterface, ClusterNetwork,
│                             VipPool, Ingress, RemoteAccessTunnel, CustomDomain,
│                             FirewallRule, TrafficPolicy
├── apps/                     AppCatalog, App, AppInstance, Vm, IsoLibrary, GpuDevice
├── crypto/                   EncryptionPolicy, KmsKey, Certificate
├── ops/                      SmartPolicy, AlertChannel, AlertPolicy, AuditPolicy, UpsPolicy,
│                             ServiceLevelObjective, ConfigBackupPolicy
└── system/                   SystemSettings, UpdatePolicy, ServicePolicy
```

## Scripts

- `pnpm --filter @novanas/schemas build` — emit `dist/`
- `pnpm --filter @novanas/schemas typecheck` — strict TS check without emit
- `pnpm --filter @novanas/schemas lint` — Biome check
