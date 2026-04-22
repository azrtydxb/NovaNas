# CRD consolidation plan

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

## References

- Source-of-truth directory:
  `packages/operators/api/v1alpha1/`
- Duplicated directory to deprecate:
  `storage/api/v1alpha1/types.go`
- CRD reference doc: [`docs/05-crd-reference.md`](05-crd-reference.md)
