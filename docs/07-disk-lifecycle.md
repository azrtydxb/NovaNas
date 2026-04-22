# 07 — Disk Lifecycle

Every physical disk is represented by a `Disk` CR with a persistent state machine. Identity is by WWN/NAA — surviving slot moves and device renames.

## States

```
UNKNOWN ──► IDENTIFIED ──► ASSIGNED ──► ACTIVE ──► DRAINING ──► REMOVABLE
                  │             │          │
                  │             │          └─► DEGRADED ──► FAILED
                  │             │                                │
                  └─► QUARANTINED ◄─────────────────────────────┘
                        │
                        └─► WIPED ──► IDENTIFIED (reuse)
```

| State | Meaning |
|---|---|
| UNKNOWN | Kernel sees a block device; not yet probed |
| IDENTIFIED | SMART/GUID/model/serial read; in UI "Available" list |
| ASSIGNED | Admin added to a StoragePool; not yet initialized |
| ACTIVE | Live member; chunk engine using it |
| DEGRADED | Chunk engine sees errors but disk still operating |
| FAILED | Engine stopped using it; recovery in progress |
| DRAINING | Admin-initiated removal; chunks migrating off |
| REMOVABLE | Empty of data; safe to physically pull |
| QUARANTINED | Taken out of pool but not wiped (recovery/forensic) |
| WIPED | Secure-erased, ready to reuse |

## Disk roles

Only two:

- `data` — participates in chunk placement
- `spare` — idle; auto-promoted on failure

There is **no** `metadata` or `cache` role — metadata is a chunk-engine-backed dataset on a tier-appropriate pool; cache is a tiering datamover relationship between pools.

## Add disk flow (hot-insert)

1. Kernel hotplug event → `novanas-disk-manager` DaemonSet (host access) detects
2. Reads WWN, model, serial, geometry, SMART, existing signatures
3. Creates `Disk` CR in IDENTIFIED state
4. UI notification: *"New 8 TB disk detected in slot 4"*
5. Admin chooses:
   - **Role**: data or spare
   - **Pool**: one of existing pools (UI filters on class match; warns on mismatch), or create new
   - **Rebalance**: immediate, later, or manual-only
6. On commit → transitions ASSIGNED → ACTIVE
7. If `rebalanceOnAdd` triggered, chunks migrate onto new disk (rate-limited by pool `recoveryRate`)

**No auto-assignment.** Even when only one pool matches.

**Foreign-pool disks**: if the inserted disk carries a NovaNas superblock from another pool:
- Offer "Import this pool" (if all disks of that pool are present)
- Offer "Salvage mode" (explicit read-only recovery; see below)
- Offer "Initialize" (wipe and add to local pool)
- Default: leave as IDENTIFIED until admin decides

## Planned removal

1. Admin clicks "Remove" → transitions ACTIVE → DRAINING
2. Chunk engine migrates chunks off, prioritizing healthy replicas
3. UI shows progress + ETA
4. **Protection-violation guard**: if removing would drop below any volume's minimum for its protection policy, UI refuses and explains
5. Drain completes → transitions to REMOVABLE
6. UI prompts: "[Blink slot LED] [Wipe] [Done]"
7. Physical pull → hotplug event → `Disk` CR kept (as "gone") for audit grace period, then deleted

## Unplanned removal (failure)

1. Failure trigger:
   - SMART pre-fail attribute crosses threshold
   - Chunk engine sees repeated I/O errors
   - Device disappears from kernel
2. Transitions ACTIVE → DEGRADED → FAILED (thresholds configurable)
3. **Dual-track recovery** (both happen in parallel):
   - **Immediate re-replication**: policy engine identifies chunks on failed disk; re-replicates/re-encodes to other healthy disks to restore target protection levels. Rate-limited by `recoveryRate`.
   - **Standby for replacement**: failed slot awaits physical replacement; new disk, when inserted, participates in rebalance and accepts its share of chunks.
4. Hot spares in the same pool are auto-promoted: engine begins rebuilding onto spare immediately, without waiting for admin action
5. Alert fired per `AlertPolicy`
6. Once protection restored (even without physical replacement), degraded state clears to normal with one-less-disk

## Emergency protection downgrade

When rebuild cannot complete due to capacity exhaustion or insufficient disks:

- Admin UI action: "Temporarily reduce protection on {volume}"
- Available modes only those achievable with current disk count
- Logged, alerted, visible on dashboard as banner
- Admin must manually upgrade back when capacity recovers

## Rebuild rate-limiting

`StoragePool.spec.recoveryRate`:

| Value | Behavior |
|---|---|
| `aggressive` | Up to 80% of disk bandwidth for rebuild |
| `balanced` (default) | ~30% of disk bandwidth; client I/O prioritized |
| `gentle` | ~10%, throttle further if client I/O spikes |

Pool can be temporarily bumped to `aggressive` when critically degraded.

## Foreign disk imports

Disks from another NovaNas box (or from this box after an OS reinstall) carry superblocks that identify their home pool.

### Strict mode (normal import)

- All disks of the foreign pool must be present
- Superblock CRUSH map validates completeness
- Encryption keys must be provided (from config backup + passphrase, or manual paste)
- On success → foreign pool + all its datasets/buckets become live
- Used for: planned hardware migration, restore-to-new-box

### Salvage mode (explicit recovery)

When disks are incomplete or pool is below protection minimum:

- Admin explicitly triggers "Salvage mode" (separate UI path from normal import)
- Scan reports: *"Pool has 4 datasets totaling 3.2 TB. Based on disks present: 2.8 TB fully recoverable, 320 GB partially recoverable, 80 GB unrecoverable."*
- Imports into a dedicated namespace `novanas-salvage/<poolname>-<timestamp>`
- All datasets/buckets mounted **read-only**
- Unreadable files return `EIO` (or placeholder, admin choice)
- To use salvaged data in normal operation, admin copies to a fresh Dataset (`novanasctl dataset copy`) — writes are re-encoded at current protection
- Status tags clearly visible: "SALVAGED — READ-ONLY — VERIFY INTEGRITY"

## SMART policy

```yaml
apiVersion: novanas.io/v1alpha1
kind: SmartPolicy
metadata: { name: default }
spec:
  appliesTo: { all: true }
  shortTest: { cron: "0 3 * * *" }          # daily
  longTest:  { cron: "0 4 * * SUN" }        # weekly
  thresholds:
    reallocatedSectors: { warning: 1, critical: 10 }
    pendingSectors:     { warning: 1, critical: 1 }
    temperature:        { warning: 50, critical: 60 }
  actions:
    onWarning: alert
    onCritical: alertAndMarkDegraded
```

## Wipe

- **Crypto erase** when chunk encryption is enabled (destroys DK) — instantaneous
- **NVMe**: `nvme format --ses=1` (crypto erase) or `--ses=2`
- **SATA SSD**: `hdparm --security-erase`
- **SATA HDD**: `blkdiscard` + single-pass zero; multi-pass "paranoid" option
- Default in UI: "Quick wipe" picks best available; "Secure wipe" forces multi-pass

## SES / slot LEDs

- Auto-detected via `/sys/class/enclosure`
- UI shows graphical enclosure view with slot positions
- Click-to-blink LED (`sg_ses`)
- Visual state: green (ACTIVE), yellow (DEGRADED), red (FAILED), blue (locator-blink), gray (empty)

## Event history

- **Last 20 events inline** on `Disk.status.recentEvents[]` (ring buffer, for UI-quick-load)
- **Full history** via Kubernetes Events (`involvedObject: Disk/<name>`)
- **Long-term retention** via `AuditPolicy` sinks (Loki, S3, syslog)

### Event schema

```go
type LifecycleEvent struct {
  Timestamp   metav1.Time
  Type        string    // Assigned | Activated | Degraded | Failed | Drained | ...
  Reason      string    // machine-readable
  Message     string    // human-readable
  FromState   DiskState
  ToState     DiskState
  Actor       string    // operator | admin:<user> | policy-engine
}
```

## Observability per disk

Prometheus metrics, graphed in UI:

- `novanas_disk_smart_temperature`
- `novanas_disk_smart_reallocated_sectors`
- `novanas_disk_smart_pending_sectors`
- `novanas_disk_smart_power_on_hours`
- `novanas_disk_chunks_hosted`
- `novanas_disk_bytes_stored`
- `novanas_disk_io_errors_total{type=read|write}`
- `novanas_disk_rebuild_progress_ratio`
- `novanas_disk_state` (enum encoded as label)

Per-disk dashboard: temperature, IOPS, latency, error counts, rebuild progress.
