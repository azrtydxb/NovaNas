# ZFS Scrub — operator runbook

NovaNAS ships an automated ZFS scrub scheduler with sensible defaults and
a REST + SDK surface for ad-hoc triggers. This document is the
day-2 reference for SREs running NovaNAS.

## What a scrub does

`zpool scrub` walks every block in a pool and verifies the on-disk
checksum against the metadata. It is the only way to detect *silent*
data corruption (bit rot, controller bugs, cosmic rays). Scrubs are an
end-to-end consistency check; SMART self-tests at the disk layer are
not a replacement.

A scrub is non-destructive: it reads, verifies, and (if redundant
metadata is available) repairs. It runs in the kernel as a background
scan and competes for IOPS with foreground I/O. ZFS deliberately
throttles a scrub when the pool is busy; expect runtime in tens of
minutes for a small SSD pool, several hours for a multi-TB HDD pool.

## Cadence recommendations

| Drive class                  | Recommended cadence       |
|------------------------------|---------------------------|
| Enterprise SSD               | Quarterly                 |
| Consumer SSD                 | Monthly                   |
| Enterprise HDD (raidz, large) | Monthly                   |
| Consumer HDD / SMR           | Bi-weekly                 |
| Cheap commodity drives       | Weekly                    |

The shipped default is **monthly, all pools, first Sunday at 02:00
local time**. The cron expression that drives it is `0 2 * * 0` with a
7-day MinFireGap, which collapses the weekly fire down to one per
month under the executor's gap-since-last-fire guard.

## Built-in default policy

On a fresh install nova-api inserts a single ScrubPolicy:

```
name:     monthly-all-pools
pools:    "*"           # expands at fire time to every imported pool
cron:     "0 2 * * 0"   # 02:00 on Sundays (first Sunday only — see below)
priority: medium
enabled:  true
builtin:  true
```

Re-running install or restarting nova-api **never** duplicates the row
(the policies table has UNIQUE(name)). Operators who want different
behaviour have two options:

1. **Edit the builtin in place** via PATCH. The `builtin: true` flag is
   advisory and does not protect the row from edit. Setting
   `enabled: false` disables it without deleting it (recommended over
   delete: a future migration that needs the row to exist will see it).
2. **Add additional policies**. Multiple enabled policies coexist
   peacefully; the per-pool dedup (`pool:<name>:scrub` UniqueKey)
   prevents two policies that both target the same pool at the same
   minute from double-dispatching.

## Policy CRUD via API

```
GET    /api/v1/scrub-policies            # list
GET    /api/v1/scrub-policies/{id}       # detail
POST   /api/v1/scrub-policies            # create (admin or operator+ with PermScrubWrite)
PATCH  /api/v1/scrub-policies/{id}       # replace mutable fields
DELETE /api/v1/scrub-policies/{id}       # remove (use with caution for builtin)
```

Body shape:

```json
{
  "name": "weekly-tank",
  "pools": "tank",
  "cron": "0 3 * * 0",
  "priority": "high",
  "enabled": true
}
```

`pools` accepts either `"*"` (all pools at fire time — new pools added
later get scrubbed automatically) or a comma-separated list of pool
names. Cron expressions are validated up front with the same parser
the snapshot scheduler uses; a malformed expression returns 400 with
the parser's error message verbatim.

## Ad-hoc trigger

Two surfaces exist for "scrub this pool now":

```
POST /api/v1/pools/{name}/scrub?action=start    # default action=start
POST /api/v1/pools/{name}/scrub?action=stop     # cancel an in-progress scrub
```

Both return 202 with a Job stub:

```json
{ "jobId": "1c2b3a4d-..." }
```

Polling status: `GET /api/v1/jobs/{jobId}`. The job tracks the
`zpool scrub` *invocation*, not the scrub itself — the kernel scan
runs asynchronously after `zpool scrub` exits and may take hours.
Track scrub progress via the metrics described below.

The Go SDK exposes these as:

```go
client.CreateScrubPolicy(ctx, novanas.ScrubPolicy{...})
client.UpdateScrubPolicy(ctx, id, novanas.ScrubPolicy{...})
client.ListScrubPolicies(ctx)
client.ScrubPool(ctx, "tank", "start")
```

## Priority

The `priority` field on a policy is **advisory** in the current
implementation. The intent (modern OpenZFS supports `zpool scrub -w`
on some versions to wait for completion, but no portable scrub
priority knob exists) is to surface the intent in metrics and audit
logs so operators can build alerts. Today it is recorded but does not
change scrub behaviour.

## Metrics

Prometheus exposes:

| Metric                                         | Type  | Labels       | Meaning |
|------------------------------------------------|-------|--------------|---------|
| `nova_zfs_scrub_last_run_timestamp_seconds`    | gauge | pool         | Unix time of last fire |
| `nova_zfs_scrub_in_progress`                   | gauge | pool         | 1 if scrubbing |
| `nova_zfs_scrub_errors_count`                  | gauge | pool         | Sum of read+write+checksum errors at leaf vdevs |
| `nova_zfs_scrub_duration_seconds`              | gauge | pool         | Wall-clock seconds since current scrub started |
| `nova_zfs_resilver_in_progress`                | gauge | pool, eta_seconds | 1 if resilvering |
| `nova_zfs_resilver_eta_seconds`                | gauge | pool         | Best-effort ETA |

A scrub running for more than 24 hours is **likely a problem**. The
recommended alert is:

```
nova_zfs_scrub_duration_seconds > 86400
```

Combine with `nova_zfs_pool_scrub_state{state="in-progress"} == 1` to
avoid firing on stale duration after a scrub completed.

## Reading scrub results

After a scrub fires, the pool's `errors_count` reflects whatever
read+write+checksum errors are accumulated on its leaf vdevs. **Any
non-zero count after a scrub is a finding**. Investigate via:

```
zpool status -v <pool>
```

The `-v` flag lists the affected files. Common scenarios:

- **Checksum errors, all on one disk**: that disk is going. Replace it.
  ZFS will resilver onto the new disk.
- **Checksum errors spread across the pool**: rare but possible — bad
  RAM, bad HBA, bad cable. Investigate the host before touching disks.
- **Read errors**: usually a cable / power / SAS expander issue. SMART
  data on the disk often confirms.
- **Write errors**: the disk is failing or is offline. Replace.

After replacing media, clear the counters with:

```
zpool clear <pool> <disk>
```

If the scrub repaired the data (i.e. ZFS used a redundant copy to fix
the corruption on disk), the file is intact. If `zpool status` reports
unrecoverable errors with file paths, those files are LOST — restore
from backup or replication.

## Resilver monitoring

Resilvers are triggered automatically by ZFS when a disk is replaced
or an offline disk comes back online. NovaNAS does not manage them
(no `zpool resilver` is needed; ZFS handles the bookkeeping) but
observes them. A resilver and a scrub are mutually exclusive in
ZFS — scrubs scheduled to fire while a resilver is in progress are
**skipped** by the executor. The resilver itself shows up via
`nova_zfs_resilver_in_progress`.

A resilver running more than 48 hours on a healthy pool is unusual;
that's the typical alert threshold.

## Recovery from scrub-found errors

Sequence:

1. **Quarantine before action** — `zpool offline <pool> <bad-disk>`
   keeps the bad disk from accumulating more errors during the
   replacement window.
2. **Pull SMART** — `nova_disk_smart_*` metrics or the
   `/api/v1/disks/{name}/smart` endpoint. Confirm the disk is
   actually failing rather than a spurious read error.
3. **Replace** — `zpool replace <pool> <bad-disk> <new-disk>` (or
   the API equivalent). ZFS resilvers automatically.
4. **Clear & re-scrub** — once resilver finishes, run a manual
   scrub (`/api/v1/pools/{name}/scrub`) to confirm the pool is clean.
5. **Update metadata** — record the disk swap in the audit log; the
   serial of the replaced disk should be searchable for warranty.

If the scrub reports unrecoverable errors:

1. Use `zpool status -v` to enumerate affected files.
2. Restore those files from the most recent good replica (snapshot,
   off-host replica, backup).
3. Run `zpool clear <pool>` once the files are restored.
4. Re-scrub to confirm.

## Skipped fires

The executor logs at INFO when it skips a pool:

- `scrubpolicy: skipping pool — scrub already running` — a previous
  fire is still in the kernel, normal for HDD pools whose monthly
  scrub takes >24 h.
- `scrubpolicy: duplicate dispatch skipped` — two policies fired the
  same minute for the same pool; the second was deduped at the job
  layer. Harmless.

Both keep the policy's `last_fired_at` updated so the next tick
respects the MinFireGap.
