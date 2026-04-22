# Performance troubleshooting

fio baselines, slow NFS/SMB throughput, latency spikes, disk
bottlenecks.

## fio below baseline

<a id="fio-below-baseline"></a>

**Symptom.** `e2e/qemu/performance/fio-baseline.sh` reports 20–40%
lower IOPS than the CI reference on identical hardware.

**Diagnose.**

```sh
# CPU frequency scaling:
cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor
# Expected: "performance". Not "powersave" or "ondemand".

# C-states — deep C-states murder small-IO latency.
cpupower idle-info

# NUMA binding — dataplane should be on the same node as the HBA.
numastat -c -m | head
lspci -vmm | grep -E 'NUMANode|Slot|NVMe|SAS'

# fio run with --latency_percentiles on the exact same job file the CI uses:
fio --output-format=json e2e/qemu/performance/baseline.fio \
    | jq '.jobs[].read, .jobs[].write'
```

**Root causes.**

1. CPU governor is not `performance`.
2. Deep C-states enabled; idle latency on first IO is huge.
3. Dataplane running on a different NUMA node than the HBA.
4. Disk firmware old and falling behind baseline.
5. fio's job file changed (check for uncommitted edits).

**Remediate.**

```sh
# Pin governor:
sudo cpupower frequency-set -g performance

# Limit C-states for latency-sensitive workloads:
sudo grub2-editenv - set kernelopts="... intel_idle.max_cstate=1"

# Pin dataplane to the HBA's NUMA node:
kubectl -n novanas-system patch deploy novanas-dataplane \
  --type=json -p='[{"op":"add","path":"/spec/template/spec/nodeSelector","value":{"novanas.io/numa":"0"}}]'
```

Re-run the baseline after each change; isolate which remediation moved
the needle before stacking more.

## Slow SMB

**Symptom.** SMB throughput saturates at ~200 MB/s on a 10 GbE link
that benchmarks at 1.1 GB/s via iperf3.

**Diagnose.**

```sh
# iperf3 to confirm the physical link is not the bottleneck:
iperf3 -c <nas-ip> -t 10 -P 4

# SMB server CPU:
kubectl -n novanas-system top pod -l app=smbserver

# SMB signing — CPU-bound crypto on old hardware.
kubectl -n novanas-system exec -it <smb-pod> -- \
  smbstatus --shares | head
```

**Root causes.**

1. SMB signing is mandatory (DC policy) and the NAS CPU is below AES-NI
   level, forcing software crypto.
2. Small file workload — SMB has per-file round-trip overhead; fix is
   client-side (larger files / different protocol).
3. Single TCP connection without SMB3 multichannel.

**Remediate.**

- Enable SMB3 multichannel on both sides; requires two NICs or RSS.
- If signing is not required by policy, disable for trusted networks:

  ```yaml
  apiVersion: novanas.io/v1alpha1
  kind: SmbServer
  spec:
    security:
      signing: optional       # instead of required
  ```

- For bulk transfer, NFS often outperforms SMB; consider a dual-mount.

## Slow NFS

**Symptom.** NFS reads are fast, writes drag or vice versa.

**Diagnose.**

```sh
# On the client:
cat /proc/mounts | grep nfs     # note wsize/rsize
nfsstat -m                      # confirm flags

# On the server:
kubectl -n novanas-system logs -l app=nfsserver --tail=200 \
  | grep -iE 'slow|retry'
```

**Root causes.**

1. `wsize`/`rsize` too small — negotiate 1 MB:
   `mount -o vers=4.2,wsize=1048576,rsize=1048576 ...`.
2. `sync` mount option — every write waits for commit. If the dataset
   has a battery-backed journal, `async` is safe.
3. The dataset sits on a cold-tier pool; bursts go to WAL but steady
   state hits spinning rust.

**Remediate.**

- Re-mount with larger wsize/rsize.
- Move hot datasets to a hot-tier pool
  ([hardware-expansion.md](../runbook/hardware-expansion.md)).
- Check the share's `ExportPolicy` for a conservative `sync` setting.

## Latency spikes

**Symptom.** p99 latency on clients spikes every N minutes; average is
fine.

**Diagnose.**

```sh
# Dataplane p99 histogram:
novanasctl perf latency <pool> --pXX 99

# Correlate with scrub / snapshot / replication schedules:
novanasctl pool get <pool> -o json | jq '.status.scrub.lastRun'
kubectl get snapshotschedules -A
```

**Root causes.**

1. Scrub runs during business hours and starves foreground IO.
2. Replication sends a full delta and hogs the network.
3. A cloud-backup job (to S3) is TLS-handshaking every chunk because
   the provider's server closes idle connections.

**Remediate.**

- Move scrub to off-hours; `SmartPolicy.spec.schedule = "0 2 * * 0"`.
- Add a bandwidth cap on the replication target.
- Enable persistent HTTP connections on the cloud backup target
  (`keepAliveSeconds: 60`).

## Disk bottlenecks

**Symptom.** Throughput is fine for reads, collapses on writes with
sustained high `%util` on one disk.

**Diagnose.**

```sh
# On the NAS host:
iostat -x 1 5 | awk 'NR==1 || /nvme|sd|vd/'
# Look for one device at 100% %util while peers idle.

# Per-chunk placement:
novanasctl chunk placement <pool> --histogram
```

**Root causes.**

1. Hot chunk on one disk; placement is not rebalancing because the
   rebalance budget is tiny.
2. A slow drive in the pool pulling down all writes (tail latency
   drags the whole stripe).
3. Firmware-level write cache disabled on just one unit.

**Remediate.**

- Increase rebalance budget:

  ```yaml
  apiVersion: novanas.io/v1alpha1
  kind: StoragePool
  spec:
    rebalance:
      maxConcurrent: 8
      maxThroughputMBps: 400
  ```

- Drain the slow drive and benchmark it standalone. If slower than
  peers, replace it.
- Verify write-cache config uniform across the pool:

  ```sh
  for d in /dev/disk/by-id/*; do
    hdparm -W $d 2>/dev/null | grep -E 'write-caching'
  done
  ```
