# Disk replacement

Replacing a disk that is failing, predicted to fail, or retired. Covers
SMART-flagged drives, the drain sequence, hot-swap procedure, and
secure wipe of the old unit.

## Signals that trigger replacement

- `SmartPolicy` controller emits event `SmartPrefailure` on the Disk.
- Prometheus alert `DiskReallocatedSectorsHigh` (> 100) or
  `DiskPendingSectorsHigh` (> 0).
- Pool rebuild backlog grows repeatedly on reads to one disk.
- Manufacturer-reported failure (RMA triggered).

Do *not* wait for the disk to hard-fail if any of the above have been
flashing for > 24h; the chunk engine can survive a double fault only
up to the pool's redundancy budget.

## Pre-flight

```sh
# Identify the failing disk.
novanasctl disk list --pool <pool> | grep -E 'Prefail|Warning'

# Confirm pool has enough redundancy headroom.
novanasctl pool get <pool> -o json | jq '.status.redundancy'
# redundancy.available must be > 0 before you start the drain.
```

If `redundancy.available == 0` STOP and add a spare first (see
`hardware-expansion.md`).

## Drain sequence

1. Mark the disk for drain:

   ```sh
   novanasctl disk drain <disk-id>
   ```

   The operator will re-place all chunks currently on this disk onto the
   remaining pool members. Expect ~1h per 2TB of live data on warm tier.

2. Watch progress:

   ```sh
   kubectl get disk <disk-id> -o jsonpath='{.status.drain}' && echo
   ```

   Wait for `phase: Drained`.

3. The pool does *not* auto-remove drained disks — they stay present but
   with zero chunks until you physically pull them.

## Hot-swap

1. Locate the bay by LED:

   ```sh
   novanasctl disk locate <disk-id> --on
   # returns the physical slot and lights the fault LED on the backplane
   ```

2. Pull the drive. The operator detects the removal within 5s and moves
   the Disk to phase `Removed`.

3. Insert the replacement. udev publishes a new Disk object; confirm:

   ```sh
   kubectl get disks -w | grep <bay-slot>
   ```

4. Add it to the pool, replacing the old UUID:

   ```sh
   kubectl edit storagepool <pool>
   # replace the drained disk's UUID with the new disk's UUID in spec.disks
   ```

5. The operator rebalances to redistribute chunks onto the new disk.

## Verification

- `novanasctl pool get <pool>` — `Ready`, `rebalance.progress == 100%`.
- `novanasctl disk list --pool <pool>` — new disk present, no
  `Warning`/`Prefail` conditions.
- Prometheus: `novanas_pool_redundancy_available` returns to its
  pre-incident value.

## Wiping the removed disk

Before sending an RMA or recycling:

```sh
# If reinstalled in the same host temporarily:
novanasctl disk wipe <disk-id> --confirm --overwrite-passes 1
```

For SEDs, issue a cryptographic erase (`sedutil-cli --revertnoerase
debug`); for non-SED SSDs use the vendor secure-erase tool; for HDDs use
the wipe subcommand above or `shred -n 1 -z /dev/sdX`. A single pass is
sufficient on modern drives — NIST 800-88.

## Gotchas

- Drain can stall if the pool redundancy is already degraded elsewhere
  (double-fault avoidance). Fix the other issue first.
- Hot-swapping during a scrub pauses the scrub; it resumes automatically
  when the new disk enters the pool.
- NVMe drives on some HBAs require a PCIe hot-remove nudge:
  `echo 1 > /sys/bus/pci/devices/<addr>/remove` before pulling.
- On SATA expander backplanes, pulling the drive without a drain may
  trip the expander's link reset; prefer the `drain → wait → pull`
  sequence even if the drive is already reporting offline.
