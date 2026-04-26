# CRD consolidation plan — SUPERSEDED

> **Status: Superseded** (2026-04-26).
>
> This document is retained for historical context only.
>
> The architectural direction has changed: NovaNas now defines **zero
> CRDs** in its own code. All resources are API-server-owned objects
> backed by Postgres; the container runtime is a swappable adapter
> (Kubernetes today, Docker planned). See
> [ADR 0005](adr/0005-hide-kubernetes-behind-api.md) and
> [05-resource-reference.md](05-resource-reference.md).
>
> Commit `80a1eef` (#52) deleted 28 CRD types that had migrated to
> Postgres. The remaining 24 (app, appinstance, blockvolume, bond,
> clusternetwork, customdomain, firewallrule, gpudevice, hostinterface,
> ingress, iscsitarget, nfsserver, nvmeoftarget, objectstore,
> physicalinterface, remoteaccesstunnel, servicepolicy, share,
> smbserver, sshkey, trafficpolicy, vippool, vlan, vm) are also slated
> for removal — see the code-removal task list referenced from
> [01-architecture-overview.md](01-architecture-overview.md). The
> "consolidate two CRD packages" framing below no longer applies; the
> work is now "delete both packages and replace with API + controller
> code".

---

Tracking document for closing issue #35. This is an *audit + plan*; the
actual refactor is deliberately left for a follow-up change because it
spans type registration, deepcopy codegen, and every controller. This
plan establishes:

1. The list of duplicated CRD types across the two `api/v1alpha1/`
   packages.
2. The designated source of truth.
3. The migration mechanics (re-exports, codegen changes, import cleanup).
4. A per-type tracking checklist.

## Scope

Two Go packages currently both register types under the
`novanas.io/v1alpha1` group:

| Package | Path |
| --- | --- |
| `storage/api/v1alpha1` | Storage-only types (StoragePool, BlockVolume, SharedFilesystem, ObjectStore, BackendAssignment, StorageQuota) |
| `packages/operators/api/v1alpha1` | The full CRD surface (disk, dataset, share, pool, plus identity, network, apps, VM, alerting, etc.) |

Historically the storage stack shipped as a separate binary with its
own scheme. As NovaNas matured the operators package absorbed the full
CRD surface and the storage package's types became a subset.

## Designated source of truth

**`packages/operators/api/v1alpha1`** is the source of truth.

Rationale:

- It already carries the full CRD surface; the storage package is a
  subset.
- The operators module is the kubebuilder module that owns deepcopy,
  conversion webhooks, and `config/crd/bases/*.yaml` generation.
- Controllers that cross storage and non-storage boundaries (e.g.,
  `ReplicationJob`, `Snapshot`) already import the operators package.

## Duplicated / overlapping types

All of the following exist in **both** packages under the same
`novanas.io/v1alpha1` group. Storage is the copy to remove; operators
is the copy to keep.

| Kind | operators type | storage type | Notes |
| --- | --- | --- | --- |
| StoragePool | `StoragePool` | `StoragePool` | Superset lives in operators (adds `tier`, `rebalance` knobs). |
| BlockVolume | `BlockVolume` | `BlockVolume` | Operators version drops legacy `FileBackendSpec`. |
| (SharedFilesystem) | `Dataset`/`Share` | `SharedFilesystem` | Storage's `SharedFilesystem` has no operators twin — split into `Dataset` + `Share` in operators. Will be renamed during migration. |
| ObjectStore | `ObjectStore` | `ObjectStore` | Straight duplicate. |
| BackendAssignment | (none) | `BackendAssignment` | Internal to storage binary; keep private to storage. |
| StorageQuota | (none) | `StorageQuota` | To be promoted to operators during this consolidation. |
| (sub-types) | `DataProtectionSpec`, `ReplicationSpec`, `EncryptionSpec`, `ErasureCodingSpec`, `DeviceFilter` | same names | Small value types reused inside Spec. Move alongside parents. |

### Not in the overlap (operators-only, keep as-is)

Everything else in `packages/operators/api/v1alpha1/`, covering disk,
dataset, share, bucket, snapshot, replication, backup, cert, network,
identity, alerting, app, vm. Nothing to do in the consolidation for
these beyond not breaking their imports.

### Not in the overlap (storage-only, keep as-is)

`BackendAssignment` — this is an internal contract between the storage
coordinator and its backend drivers; no reason to expose it above the
storage binary. Stays in `storage/api/v1alpha1`, not promoted.

## Migration mechanics

1. **Promote storage-only types that must be cluster-wide** (just
   `StorageQuota`). Copy the type into
   `packages/operators/api/v1alpha1/storagequota_types.go`, regenerate
   deepcopy and CRD yaml.

2. **Convert storage's duplicated types into re-exports**. Example:

   ```go
   // storage/api/v1alpha1/storagepool_reexport.go
   package v1alpha1

   import operatorsapi "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"

   // Type aliases: no conversion needed, identity with the operators type.
   type StoragePool      = operatorsapi.StoragePool
   type StoragePoolSpec  = operatorsapi.StoragePoolSpec
   type StoragePoolStatus = operatorsapi.StoragePoolStatus
   type StoragePoolList  = operatorsapi.StoragePoolList
   ```

   Caveats:
   - Type aliases cannot add methods. The existing `DeepCopyInto`
     methods on storage types must move with the type (operators
     package already has them).
   - Kubebuilder markers (`+kubebuilder:object:root=true`) on the
     re-exported alias are ignored; registration happens in the
     operators scheme only. Storage's scheme registers aliases for
     source compatibility, not new CRDs.

3. **Update the storage binary's scheme**. Replace
   `AddToScheme(novanasv1alpha1.AddToScheme)` (local) with
   `operatorsapiv1alpha1.AddToScheme`. This removes the risk of two
   different `DeepCopy` implementations drifting.

4. **Delete duplicate deepcopy output**. After re-exports land,
   `zz_generated.deepcopy.go` in storage is regenerated to contain
   only the types that actually live in storage (`BackendAssignment`
   and any small helpers).

5. **Collapse CRD manifests**. `storage/config/crd/bases/*.yaml` for
   duplicated Kinds are deleted; the operators chart is the only
   source.

6. **Controller imports**. Any `storage/...` code that imports the
   local v1alpha1 keeps compiling thanks to the aliases. New code
   should import `packages/operators/api/v1alpha1` directly.

7. **Helm chart**. `helm/charts/storage` drops the CRD install block;
   the umbrella chart already installs CRDs from the operators
   package.

8. **Deprecation window**. Keep the aliases for one minor release so
   out-of-tree controllers (if any) have time to migrate imports.
   Remove in the release after.

## Tracking checklist

Use a separate PR per item where possible; CRD shape changes should
never ride along with unrelated refactors.

- [ ] Promote `StorageQuota` into operators (types + deepcopy + CRD yaml)
- [ ] Re-export `StoragePool`, `StoragePoolSpec`, `StoragePoolStatus`,
      `StoragePoolList`, `FileBackendSpec`, `DeviceFilter`,
      `DataProtectionSpec`, `ReplicationSpec`, `EncryptionSpec`,
      `ErasureCodingSpec` from storage → operators
- [ ] Re-export `BlockVolume`, `BlockVolumeSpec`, `BlockVolumeStatus`,
      `BlockVolumeList` from storage → operators
- [ ] Re-export `ObjectStore`, `ObjectStoreSpec`, `ObjectEndpointSpec`,
      `ObjectServiceSpec`, `BucketPolicySpec`, `ObjectStoreStatus`,
      `ObjectStoreList` from storage → operators
- [ ] Decide fate of `SharedFilesystem`: (a) rename to
      `Dataset`+`Share` in storage and re-export, or (b) mark as
      deprecated alias for operators' `Dataset`
- [ ] Update `storage/cmd/.../main.go` to register the operators scheme
- [ ] Delete duplicated `zz_generated.deepcopy.go` entries in storage
- [ ] Delete duplicated CRD manifests in `storage/config/crd/bases/`
- [ ] Update `helm/charts/storage/templates/` — remove duplicate CRD
      installs
- [ ] Grep for `storage/api/v1alpha1` across the tree; update imports
      that should now go to operators directly
- [ ] Release note: "internal CRD types consolidated; no API-level
      change for cluster operators"
- [ ] Remove the aliases one release later

## Non-goals

- No wire-level changes. Existing CRDs on-cluster continue to work;
  no conversion webhooks required.
- No API version bump.
- No controller-logic changes.
- No rename of `novanas.io/v1alpha1`. We may revisit v1 promotion
  after consolidation lands.

## Executed (2026-04-22, issue #35)

C2 consolidation pass outcome:

- **Storage module plumbing:** added
  `replace github.com/azrtydxb/novanas/packages/operators => ../packages/operators`
  to `storage/go.mod`. `go mod tidy` stays clean; `go build ./...` in the
  storage module is green. The replace directive is dormant (no storage
  code imports the operators API package yet) but unblocks a follow-up
  that actually performs the aliasing.

- **Aliasing deferred by design:** per the plan's own risk note ("don't
  force a consolidation that breaks semantics"), the duplicated types
  are *not* converted into Go type aliases in this pass. On inspection
  the two packages have diverged along semantic axes, not just extra
  fields:

  | Kind | Divergence |
  | --- | --- |
  | StoragePool | storage's Spec is a backend contract (NodeSelector/BackendType/FileBackend); operators' Spec is a policy contract (Tier/RecoveryRate/RebalanceOnAdd). No field overlap beyond DeviceFilter. |
  | DeviceFilter | storage: Type/MinSize. operators: PreferredClass/MinSize/MaxSize. Different field names *and* semantics. |
  | BlockVolume | storage: AccessMode, DataProtection, Quota. operators: Protection, Tiering. Different names, different shapes. |
  | ErasureCodingSpec | storage uses `int`; operators uses `int32`. Go type aliases require structural identity — aliasing is impossible without a breaking field-type change on one side. |
  | ObjectStore | operators' Spec is an empty TODO stub; aliasing would erase the concrete Endpoint/BucketPolicy fields storage consumers rely on. |
  | SharedFilesystem | no operators twin — operators split this concept into Dataset + Share with a different export model. Not aliasable. |
  | BackendAssignment | storage-internal, correctly kept storage-local per the original plan. |
  | StorageQuota | storage-only; operators promotion not executed in this pass (would require regenerating operators' CRD yaml + deepcopy, out of scope for the C2 bounded refactor). |

  Forcing an alias in this state would require either (a) a breaking,
  non-additive rewrite of the operators schema (outside this agent's
  ownership — operators internal controllers consume the existing
  fields), or (b) breaking every storage controller that reads
  `Spec.BackendType`, `Spec.FileBackend`, `Spec.NodeSelector`,
  `Spec.AccessMode`, etc. Neither is acceptable.

- **Doc comment in `storage/api/v1alpha1/types.go`:** the package now
  carries a top-of-file comment summarising the divergence so future
  maintainers don't re-open the same investigation.

- **Checklist status after this pass:**
  - [ ] Promote `StorageQuota` into operators — *deferred (needs
        operators CRD + deepcopy regeneration)*.
  - [ ] Re-export StoragePool / BlockVolume / ObjectStore /
        SharedFilesystem families — *deferred; see divergence table
        above. Requires a field-by-field reconciliation pass.*
  - [x] `storage/go.mod` replace directive pointing at
        `../packages/operators` — added.
  - [x] `storage/api/v1alpha1` documented as the not-yet-consolidated
        subset.
  - [ ] Remaining checklist items (scheme swap, delete duplicate
        deepcopy / CRD manifests, helm chart changes, import greps)
        intentionally left for the follow-up pass that actually lands
        the reconciled types.

- **Recommended follow-up:** a *single focused PR per type family*
  that first lands the needed additive changes on the operators side
  (e.g., extending `StoragePoolSpec` with the backend-contract fields
  currently storage-local), then converts storage's copy into an
  alias, then removes the storage-local deepcopy entries. Bundling
  them all in one pass is what made C2 risky; splitting by kind keeps
  each change reviewable.

## References

- Source-of-truth directory:
  `packages/operators/api/v1alpha1/`
- Duplicated directory to deprecate:
  `storage/api/v1alpha1/types.go`
- CRD reference doc: [`docs/05-crd-reference.md`](05-crd-reference.md)
