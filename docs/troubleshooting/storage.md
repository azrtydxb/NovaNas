# Storage troubleshooting

Chunk engine, rebuilds, scrubs, WAL corruption, replication stalls.

## Rebuild stuck at 99%

**Symptom.** `novanasctl pool get <pool>` shows
`status.rebuild.progress` pinned at 99% for more than an hour.

**Diagnose.**

```sh
novanasctl pool get <pool> -o json | jq '.status.rebuild'
kubectl -n novanas-system logs -l component=chunk-engine \
  --tail=500 | grep -E 'chunk=[a-f0-9]+ unreadable|rebuild'
```

Look for repeated `chunk=<id> unreadable` entries for the same chunk
ID.

**Root causes.**

1. A single chunk on the draining disk is unreadable *and* has fewer
   live replicas than the rebuild target.
2. A replacement disk rejoined mid-rebuild and is itself failing
   reads (SMART stats).
3. WAL replay is blocked by a torn write on a failing drive.

**Remediate.**

- Find which disks host the stuck chunk:

  ```sh
  novanasctl chunk locate <chunk-id>
  ```

- If one of those disks has SMART warnings, drain it per
  [disk-replacement.md](../runbook/disk-replacement.md). The rebuild
  will re-route.
- If all replicas are unreadable, the data is lost. Identify affected
  files:

  ```sh
  novanasctl chunk files <chunk-id>
  ```

  Restore those files from the most recent snapshot or replica.

## Scrub checksum errors

**Symptom.** `ScrubSchedule` emits `ChecksumMismatch` events; pool
`status.scrub.errors > 0`.

**Diagnose.**

```sh
kubectl get events -A --field-selector reason=ChecksumMismatch \
  --sort-by=.lastTimestamp | tail -20

novanasctl pool get <pool> -o json \
  | jq '.status.scrub'
```

Identify the affected disk(s):

```sh
novanasctl disk list --pool <pool> -o json \
  | jq '.items[] | select(.status.scrub.errors>0) | .metadata.name'
```

**Root causes.**

1. Failing drive — correlate with SMART reallocated/pending sectors.
2. Bad SAS/SATA cable — errors cluster on one bay group or one HBA
   port.
3. Bad RAM — errors scatter across pools/disks. Check `edac-util`.
4. Firmware bug on the HBA — rare but catastrophic.

**Remediate.**

- Drive: drain and replace
  ([disk-replacement.md](../runbook/disk-replacement.md)).
- Cable: reseat, re-run scrub; if errors persist replace the cable.
- RAM: reboot into memtest; replace the stick.
- HBA: update firmware from the vendor's supported list only.

Re-run the scrub after each remediation:

```sh
novanasctl pool scrub <pool>
```

## Open-chunk WAL corruption

**Symptom.** Dataplane log emits `wal: truncated record at offset
<N>`; shares stay read-only.

**Diagnose.**

```sh
kubectl -n novanas-system logs -l component=dataplane \
  --tail=500 | grep -E 'wal:|open-chunk'
```

Check the WAL tail:

```sh
# On the node holding the pool primary:
sudo novanasctl debug wal inspect /var/lib/novanas/pool/<pool>/wal
```

**Root causes.**

1. Power loss during a write with disk write-cache misconfigured
   (caches were enabled without BBU).
2. Kernel panic during write; torn record at end of WAL.
3. Filesystem under /var/lib/novanas is full.

**Remediate.**

- Torn tail record: the dataplane truncates and replays safely. Just
  restart the dataplane pod:

  ```sh
  kubectl -n novanas-system rollout restart deploy/novanas-dataplane
  ```

- Full filesystem: free space, ideally by pruning old logs. The WAL
  itself is fixed-size ring — the overflow is elsewhere.
- If `wal inspect` shows multiple torn records in the middle of the
  log, the WAL is not safely replayable. You must rewind to the last
  checkpoint — this loses the in-flight writes:

  ```sh
  sudo novanasctl debug wal rewind /var/lib/novanas/pool/<pool>/wal \
    --to-checkpoint --confirm
  ```

  Open a support case; this should never happen without a catastrophic
  power event.

## Replication stuck

**Symptom.** A `ReplicationJob` has been in `Running` for longer than
the interval between runs; `status.progress` not advancing.

**Diagnose.**

```sh
novanasctl replication job get <job> -o json \
  | jq '{phase:.status.phase,lag:.status.lag,bytesSent:.status.bytesSent,updatedAt:.status.updatedAt}'

kubectl -n novanas-system logs deploy/novanas-replication \
  --tail=200 | grep -F <job-name>
```

**Root causes.**

1. Snapshot pruning race: the remote side deleted the parent snapshot
   the incremental was based on.
2. Bandwidth limit set too low for the day's change rate.
3. Target unreachable (firewall change, WireGuard key rotated on one
   side only).

**Remediate.**

- Parent-snapshot race:

  ```sh
  novanasctl replication job reset <job>
  # Forces a new base snapshot. Next run re-sends a full; be patient.
  ```

- Bandwidth: raise `ReplicationTarget.spec.bandwidthLimitMbps`.
- Unreachable: the remote side's audit log will show the specific
  failure. Fix the transport; the job retries automatically.
