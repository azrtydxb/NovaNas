# Hardware expansion

Adding storage capacity — either new disks to an existing pool or a
fresh pool — without downtime.

## When to expand

- Pool usage > 75% (alerted by `PoolCapacityWarning`).
- You are adding a new workload tier (e.g., NVMe for a hot dataset).
- You need more IOPS; an existing pool is IOP-bound but not
  capacity-bound.

Do *not* expand to work around SMART errors — replace the failing disk
first (see `disk-replacement.md`).

## Capacity planning

1. Pull the last 30 days of usage:

   ```sh
   novanasctl pool get <pool> -o json | jq '.status.usage'
   ```

2. Project linear growth over the next 90 days; target < 70% after
   expansion. Anything denser than that risks fragmentation and scrub
   time creep.

3. Match the new disks to the pool's existing tier. Mixing NVMe and SATA
   in the same chunk group degrades to SATA speeds.

## Picking a pool tier

| Workload | Tier | Disk class |
| --- | --- | --- |
| VM disks, app databases | `tier-hot` | NVMe U.2 (enterprise) |
| General shares, home dirs | `tier-warm` | SATA SSD |
| Backups, archives | `tier-cold` | SATA HDD |
| Object store backing | `tier-cold` | HDD, erasure-coded |

Tiers are enforced via `StoragePool.spec.tier` and pool-scoped scheduler
policies.

## Procedure — add disks to an existing pool

1. Physically install the disk(s). Verify detection:

   ```sh
   kubectl get disks
   novanasctl disk list --pool <pool>
   ```

   New disks should appear within 30s (udev triggers Disk objects).

2. Wipe any residual partition tables *only* if the system did not
   auto-detect them as blank:

   ```sh
   novanasctl disk wipe <disk-id> --confirm
   ```

3. Add disks to the pool:

   ```sh
   kubectl edit storagepool <pool>
   # append the new disk UUIDs to spec.disks[]
   ```

4. Watch the rebalance:

   ```sh
   novanasctl pool get <pool> -w
   ```

   `status.rebalance.progress` should climb monotonically; expect ~1h per
   TB on warm tier, ~15 min per TB on hot.

5. Confirm:

   ```sh
   novanasctl pool get <pool> -o json | jq '.status.capacity'
   ```

## Procedure — create a new pool

1. Identify candidate disks:

   ```sh
   novanasctl disk list --unassigned
   ```

2. Author the pool spec (`pool-hot.yaml`):

   ```yaml
   apiVersion: novanas.io/v1alpha1
   kind: StoragePool
   metadata: { name: hot-nvme }
   spec:
     tier: tier-hot
     redundancy: { parity: 1, copies: 1 }
     disks: [disk-00xx, disk-00yy, disk-00zz]
   ```

3. Apply and wait for `Ready`:

   ```sh
   kubectl apply -f pool-hot.yaml
   novanasctl pool get hot-nvme -w
   ```

4. Point a dataset at it:

   ```sh
   kubectl patch dataset <dataset> --type merge \
     -p '{"spec":{"pool":"hot-nvme"}}'
   ```

## Verification

- `novanasctl pool list` — new/extended pool shows `Ready`.
- Prometheus: `novanas_pool_capacity_bytes` jumps by the added size.
- `novanasctl disk list --pool <pool>` lists all expected disks.

## Rollback

Adding disks is reversible via `novanasctl pool remove-disk`, but only
before data is placed on the new disks. Once rebalance has written
chunks, removal requires a full drain (same flow as `disk-replacement`).

## Gotchas

- Adding a disk smaller than the pool's existing disks caps the whole
  pool's new capacity; the operator warns but does not block.
- Hot-add across NUMA nodes is fine on AMD EPYC and recent Intel; on
  older systems pin dataplane cores to the same NUMA node as the HBA.
- Scrub scheduling resets to the pool's default after any disk add;
  re-apply `SmartPolicy` explicitly if you override it.
