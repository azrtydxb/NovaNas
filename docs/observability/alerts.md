# NovaNAS Alert Catalog

This catalog documents every Prometheus alert rule shipped with NovaNAS. Each
entry includes the severity, what triggers it, why it matters, and a runbook
the operator can follow when it fires.

## Severity model

| Severity | Routing | Meaning |
|----------|---------|---------|
| `critical` | page on-call immediately (page-team receiver) | Production impact now or imminent. Operator response within minutes. |
| `warning`  | email-team within ~hour (email-team receiver) | Degradation or precursor to a critical. Operator response within a working day. |
| `info`     | daily digest (digest-team receiver) | Awareness signal; no immediate action expected. |

## Common labels

All alerts carry the following labels:

| Label | Source | Used for |
|-------|--------|----------|
| `severity` | rule label | routing |
| `component` | rule label (`zfs`, `disk`, `host`, `network`, `nova-api`, `openbao`, `keycloak`, `postgres`, `redis`, `kubevirt`, `replication`, `filesystem`) | grouping + per-component routes |
| `runbook_url` | rule label | direct link to the section below |
| `cluster`, `instance` | Prometheus external_labels / scrape | dedup, inhibition |

Alertmanager inhibition rules in `deploy/alertmanager/alertmanager.yml.example`
ensure the worst symptom always fires alone (e.g. `ZFSPoolFaulted` mutes
`ZFSVdev*` alerts on the same pool).

## Resolved-alert behavior

All alerts are stateless: when the underlying expression goes false, Prometheus
emits a `resolved` event and Alertmanager forwards it to the receiver
(`send_resolved: true` in receiver configs). There are no manual ack-then-stale
gotchas — the alert simply disappears once the condition clears.

## Rule files

| File | Purpose |
|------|---------|
| `recording.yml` | Base recording rules referenced by ≥3 alerts |
| `zfs.yml`       | Pool / vdev / scrub / collector |
| `disks.yml`     | SMART + filesystem capacity |
| `system.yml`    | Host load, memory, network, time |
| `api.yml`       | nova-api 5xx, latency, jobs, replication |
| `services.yml`  | OpenBao, Keycloak, Postgres, Redis, KubeVirt VMs |

---

## ZFS

### ZFSPoolFaulted
- **Severity:** critical
- **Summary:** ZFS pool reports `state=FAULTED`.
- **Why it matters:** Pool is unusable; data is at risk.
- **First check:** `zpool status -v <pool>` to see which vdev/disk is faulted.
- **Fix:** Replace failed devices, then `zpool clear <pool>`. If the pool is
  unrecoverable, restore from replication target.

### ZFSPoolDegraded
- **Severity:** critical
- **Summary:** Pool is `DEGRADED` (vdev offline or erroring).
- **Why it matters:** Redundancy is reduced; one more failure may fault the pool.
- **First check:** `zpool status -v <pool>`; identify the failing vdev/disk.
- **Fix:** Replace the device with `zpool replace <pool> <old> <new>`; wait for
  resilver to finish.

### ZFSPoolUnavail
- **Severity:** critical
- **Summary:** Pool reports `UNAVAIL`, `REMOVED`, or `SUSPENDED`.
- **Why it matters:** Pool is not serving I/O.
- **First check:** `dmesg | grep -i zfs`, then `zpool status`.
- **Fix:** Reattach hardware if removed, run `zpool clear` once recoverable. For
  SUSPENDED pools, address the underlying I/O failure first.

### ZFSPoolCapacityWarn
- **Severity:** warning
- **Summary:** Pool capacity > 85%.
- **Why it matters:** ZFS write performance degrades sharply above ~80%.
- **First check:** `zfs list -t snapshot -o name,used -s used <pool>`.
- **Fix:** Delete old snapshots, prune datasets, or add a vdev.

### ZFSPoolCapacityCritical
- **Severity:** critical
- **Summary:** Pool capacity > 95%.
- **Why it matters:** Writes will start failing imminently.
- **First check:** Same as warn, plus `zpool list -v <pool>`.
- **Fix:** Free space immediately or attach an additional vdev. Stop
  non-essential writers (Loki retention shrink, snapshot purge).

### ZFSPoolFragmentationHigh
- **Severity:** warning
- **Summary:** Pool fragmentation > 50% for 1h.
- **Why it matters:** Sequential write throughput is reduced.
- **First check:** `zpool list -v <pool>` for FRAG column per vdev.
- **Fix:** Plan a data rebalance; `zfs send | zfs receive` to a freshly
  allocated dataset, or add fresh vdevs.

### ZFSVdevReadErrors / ZFSVdevWriteErrors / ZFSVdevChecksumErrors
- **Severity:** warning
- **Summary:** Non-zero error counter on a leaf vdev for >10m.
- **Why it matters:** Almost always a failing disk or bad cable/controller.
- **First check:** `zpool status -v <pool>`, then SMART on the indicated path.
- **Fix:** Replace the disk if SMART shows failure markers; otherwise inspect
  cabling/controller. Run a scrub afterwards to verify integrity.

### ZFSScrubMissing
- **Severity:** warning
- **Summary:** A pool has not had a scrub recorded for 30 days.
- **Why it matters:** Scrubs detect and repair latent bit-rot.
- **First check:** `zpool history <pool> | grep -i scrub`.
- **Fix:** `zpool scrub <pool>`; install a monthly systemd timer if missing.

### ZFSResilverStuck
- **Severity:** critical
- **Summary:** Resilver state has been continuous for >24h.
- **Why it matters:** Likely another failing disk thrashing the resilver.
- **First check:** `zpool status <pool>` for additional READ/WRITE/CKSUM errors.
- **Fix:** Replace the additional failing disk, then let resilver complete.

### ZFSCollectorErrors
- **Severity:** warning
- **Summary:** nova-api ZFS collector is failing polls.
- **Why it matters:** ZFS metrics are stale; downstream alerts may not fire.
- **First check:** `journalctl -u nova-api | grep "zfs metrics"`.
- **Fix:** Verify `zfs`/`zpool` binaries are present and that nova-api has
  permission to invoke them.

---

## Disks

### DiskReallocatedSectorsIncreasing
- **Severity:** warning
- **Summary:** SMART realloc count grew in the last 6h.
- **Why it matters:** Strong predictor of imminent disk failure.
- **First check:** `smartctl -a /dev/<dev>`.
- **Fix:** Schedule replacement; for ZFS pool members, `zpool replace`
  proactively.

### DiskPendingSectors
- **Severity:** warning
- **Summary:** SMART pending-sector count > 0.
- **Why it matters:** Sectors that returned bad reads are queued for
  reallocation.
- **First check:** `smartctl -a /dev/<dev>`.
- **Fix:** Run a ZFS scrub (forces rewrites). If pending stays > 0 after a
  scrub, replace the disk.

### DiskOfflineUncorrectable
- **Severity:** critical
- **Summary:** SMART offline-uncorrectable > 0.
- **Why it matters:** Data has already been lost at the device level.
- **First check:** `smartctl -a /dev/<dev>`.
- **Fix:** Replace the disk immediately; trigger `zpool replace` for pool
  members.

### SMARTHealthFailing
- **Severity:** critical
- **Summary:** Drive's overall SMART health flag = FAILING.
- **Why it matters:** The drive's own self-assessment says it's about to die.
- **First check:** `smartctl -H /dev/<dev>`.
- **Fix:** Replace immediately.

### FilesystemAlmostFull
- **Severity:** warning
- **Summary:** Mountpoint has < 10% free space for 15m.
- **Why it matters:** Prometheus, Loki, Postgres misbehave near the boundary.
- **First check:** `df -h <mp>`, `du -sh <mp>/*`.
- **Fix:** Trim retention buffers, expand the underlying dataset, or move data.

### FilesystemCritical
- **Severity:** critical
- **Summary:** Mountpoint has < 5% free space.
- **Why it matters:** Writes start failing; Postgres may shut down.
- **First check:** Same as above.
- **Fix:** Free space immediately.

### FilesystemReadOnly
- **Severity:** critical
- **Summary:** A non-tmpfs filesystem is mounted read-only.
- **Why it matters:** Linux remounts ext4/xfs r/o on persistent I/O errors.
- **First check:** `dmesg | grep -i error`.
- **Fix:** Replace failing disk; remount r/w with `mount -o remount,rw <mp>`.

---

## System

### HostDown
- **Severity:** critical
- **Summary:** node_exporter / prometheus self-scrape failing > 2m.
- **Why it matters:** No visibility into the host; could be powered off.
- **First check:** Ping the host, attempt SSH, check console.
- **Fix:** Power-cycle if needed; restart node_exporter once back.

### HighLoad
- **Severity:** warning
- **Summary:** 1-min load > number of CPU cores for 15m.
- **Why it matters:** Tasks queueing → latency rising for everything.
- **First check:** `top`, `pidstat`.
- **Fix:** Identify and throttle the noisy process; scale out if structural.

### HighMemoryPressure
- **Severity:** warning
- **Summary:** MemAvailable / MemTotal < 10% for 10m.
- **Why it matters:** ARC squeezed; OOM-killer approaches.
- **First check:** `top`, `smem -kt`.
- **Fix:** Restart leaky service; tune `zfs_arc_max`; add RAM.

### HighSwapUsage
- **Severity:** warning
- **Summary:** Swap > 50% used for 30m.
- **Why it matters:** Heavy thrashing; latency hit + likely OOM precursor.
- **First check:** `swapon --show`, `top` ordered by SWAP.
- **Fix:** Restart memory hogs; lower ARC budget; add RAM.

### TimeDriftHigh
- **Severity:** warning
- **Summary:** Clock offset > 500ms for 10m.
- **Why it matters:** Breaks Kerberos, JWT validation, replication scheduling.
- **First check:** `timedatectl status`.
- **Fix:** Restart `systemd-timesyncd` or `chronyd`; verify NTP reachability.

### NetworkInterfaceDown
- **Severity:** warning
- **Summary:** A physical interface is administratively or operationally down.
- **Why it matters:** May break service traffic, replication, cluster heartbeat.
- **First check:** `ip link`, switch port, cable.
- **Fix:** `ip link set <dev> up`; investigate cable/switch if it stays down.

---

## nova-api

### NovaAPIDown
- **Severity:** critical
- **Summary:** Prometheus cannot scrape nova-api for > 2m.
- **Why it matters:** Management plane offline; UI/CSI broken.
- **First check:** `systemctl status nova-api`, `journalctl -u nova-api`.
- **Fix:** Restart; check TLS material on :8444; verify CA at
  `/etc/nova-ca/ca.crt`.

### NovaAPIHigh5xxRate
- **Severity:** critical
- **Summary:** Global 5xx rate > 5% for 10m with non-trivial traffic.
- **Why it matters:** UI / CSI / replication clients all see failures.
- **First check:** nova-api logs for stack traces.
- **Fix:** Inspect dependencies (Postgres, OpenBao, Redis, ZFS binaries).

### NovaAPIRouteHigh5xxRate
- **Severity:** warning
- **Summary:** Per-route 5xx rate > 10% for 15m.
- **Why it matters:** A specific endpoint is broken.
- **First check:** nova-api logs filtered by `path` label.
- **Fix:** Inspect the dependency the handler talks to.

### NovaAPILatencyHigh
- **Severity:** warning
- **Summary:** Per-route p95 latency > 2s for 15m on a route with traffic.
- **Why it matters:** UI sluggish, CSI may time out.
- **First check:** Look for slow `zpool`/`zfs` invocations or DB latency.
- **Fix:** Profile with pprof; bound shell-out concurrency.

### NovaJobsHighFailureRate
- **Severity:** warning
- **Summary:** > 10% of jobs of a given kind are failing for 10m.
- **Why it matters:** Snapshots, replications, admin tasks not completing.
- **First check:** nova-api worker logs for the failing kind.
- **Fix:** Address ZFS / Postgres / network root cause.

### NovaJobsBacklogGrowing
- **Severity:** warning
- **Summary:** > 10 jobs of a kind are in flight for 30m.
- **Why it matters:** A worker is hung (stuck zfs subprocess, DB lock).
- **First check:** `pstree` for stuck zfs processes; `pg_stat_activity`.
- **Fix:** Kill the stuck subprocess; if persistent, restart nova-api.

### NovaReplicationJobFailing
- **Severity:** critical
- **Summary:** A replication job has failed its recent runs (placeholder
  metric: `nova_replication_last_run_status` — wired now, will activate when
  the replication subsystem publishes it).
- **Why it matters:** Off-site copy is stale; RPO is silently growing.
- **First check:** nova-api replication logs; destination host network/zfs
  receive state.
- **Fix:** Address the destination-side issue; re-run the job manually.

---

## Services

### OpenBaoSealed
- **Severity:** critical
- **Summary:** OpenBao reports `vault_core_unsealed == 0`.
- **Why it matters:** No service can read secrets; logins will fail.
- **First check:** `bao status`.
- **Fix:** Run `nova-bao-unseal` or `bao operator unseal` with the unseal
  shards.

### OpenBaoDown
- **Severity:** critical
- **Summary:** Prometheus cannot scrape OpenBao for > 2m.
- **Why it matters:** Same blast radius as Sealed.
- **First check:** `systemctl status openbao`, TLS on :8200.
- **Fix:** Restart; check certs.

### KeycloakDown
- **Severity:** critical
- **Summary:** Probe or scrape against Keycloak failing for > 3m.
- **Why it matters:** All SSO logins (Grafana, oauth2-proxy, Prometheus UI)
  break.
- **First check:** `systemctl status keycloak`, OIDC discovery URL.
- **Fix:** Restart; validate realm import; check Postgres for Keycloak.

### PostgresDown
- **Severity:** critical
- **Summary:** postgres-exporter scrape failing for > 2m.
- **Why it matters:** nova-api persistence + Keycloak both depend on Postgres.
- **First check:** `systemctl status postgresql`, dataset disk space.
- **Fix:** Restart; check WAL/data dir space; verify exporter credentials.

### PostgresTooManyConnections
- **Severity:** warning
- **Summary:** Connection count > 90% of `max_connections` for 10m.
- **Why it matters:** New clients (including nova-api) will be refused soon.
- **First check:** `SELECT * FROM pg_stat_activity ORDER BY state, query_start;`.
- **Fix:** Find leaks; bump `max_connections`; reduce idle_in_transaction.

### RedisDown
- **Severity:** warning
- **Summary:** redis-exporter scrape failing for > 2m.
- **Why it matters:** asynq job queue stops working; nova-api cannot dispatch.
- **First check:** `systemctl status redis`.
- **Fix:** Restart; verify auth file.

### KubeVirtVMCrashLoop
- **Severity:** warning
- **Summary:** A KubeVirt VirtualMachineInstance pod has been in
  CrashLoopBackOff for > 5m.
- **Why it matters:** VM is offline.
- **First check:** `kubectl describe pod -n <ns> <pod>`,
  `kubectl logs -n <ns> <pod> -c compute`.
- **Fix:** Inspect libvirt logs; resolve the underlying domain failure.

---

## Operating the alert pipeline

- Validate rules locally:

  ```sh
  promtool check rules deploy/prometheus/rules/*.yml
  ```

- Validate routing/inhibition:

  ```sh
  amtool check-config deploy/alertmanager/alertmanager.yml.example
  ```

- After editing rules on the dev box: `sudo systemctl reload prometheus`
  (the `setup.sh` step that installs the rules also issues the reload).

- Silencing during maintenance: `amtool silence add alertname=XYZ -d 1h`.
