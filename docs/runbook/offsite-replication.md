# Off-site replication

Set up asynchronous replication to a remote NovaNas (or S3-compatible
backup target), throttle it to fit available bandwidth, and verify that
the remote copy is recoverable.

## Architectures

1. **NovaNas-to-NovaNas** — dataset-level replication over WireGuard
   between two clusters. Uses native snapshot streams.
2. **NovaNas-to-S3** — snapshot-based cloud backup. See
   `CloudBackupTarget` + `CloudBackupJob`.

This runbook covers both; pick the section that matches your target.

## Prerequisites

- Network path from site A → site B open for UDP/51820 (WireGuard) or
  HTTPS/443 (S3).
- Enough upstream bandwidth for your RPO. Rule of thumb:
  `daily_change_bytes / (bandwidth_bps / 10)` < 24h. Build in 10x for
  peak/retry headroom.
- Remote site has free capacity ≥ primary dataset size × retention
  factor (≥ 2 for weekly backups).

## NovaNas-to-NovaNas — ReplicationTarget

1. On the **remote** cluster, create a receiver user + API token
   scoped to ingest only:

   ```sh
   novanasctl user create replica-receiver --role replication-receiver
   novanasctl token create replica-token --user replica-receiver \
     --scopes dataset.receive --out /tmp/token
   ```

2. On the **primary** cluster, create the target:

   ```yaml
   apiVersion: novanas.io/v1alpha1
   kind: ReplicationTarget
   metadata: { name: dr-site }
   spec:
     endpoint: https://nas-dr.example.com
     authSecretRef: { name: replica-token }
     bandwidthLimitMbps: 200          # throttle to 200 Mbps
     transport: wireguard
     tlsInsecureSkipVerify: false
   ```

   ```sh
   kubectl create secret generic replica-token \
     --from-file=token=/tmp/token -n novanas-system
   kubectl apply -f dr-site.yaml
   ```

3. Verify handshake:

   ```sh
   novanasctl replication target get dr-site
   # status.phase → Ready within ~30s
   ```

4. Create a replication job per dataset:

   ```yaml
   apiVersion: novanas.io/v1alpha1
   kind: ReplicationJob
   metadata: { name: home-to-dr }
   spec:
     dataset: home
     target: dr-site
     schedule: "0 */6 * * *"          # every 6h
     retention: { count: 56 }         # ~2 weeks of 6h snapshots
   ```

## NovaNas-to-S3 — CloudBackupTarget

```yaml
apiVersion: novanas.io/v1alpha1
kind: CloudBackupTarget
metadata: { name: s3-dr }
spec:
  provider: s3
  endpoint: https://s3.us-east-1.example.com
  bucket: novanas-backups
  credentialsSecretRef: { name: s3-dr-creds }
  encryption: { kmsKeyRef: { name: backup-key } }
  objectLock: compliance
```

Object Lock is strongly recommended — see `ransomware-response.md`.

## Bandwidth throttling

- Per-target: `ReplicationTarget.spec.bandwidthLimitMbps` — hard cap.
- Per-schedule: use a `TrafficPolicy` gate on the Job's service account
  if you need time-of-day shaping (e.g., full rate overnight, 50%
  daytime).
- Global: `SystemSettings.spec.network.wanEgressMbps` is a safety cap
  across all targets.

Start conservative (10–25% of upstream). Raise only after a full cycle
completes without saturating other traffic.

## Verification

Run a recovery drill once per quarter:

1. Pick a non-critical dataset.
2. Restore from the replica into a scratch target:

   ```sh
   novanasctl replication restore home \
     --target dr-site \
     --into home-drill-$(date +%F)
   ```

3. Mount the restored dataset read-only; checksum a handful of files
   against the primary.
4. Delete the scratch dataset when done.

Metrics to watch:

- `novanas_replication_lag_seconds` — should stay ≤ RPO.
- `novanas_replication_bytes_sent_total` — monotonic increase per run.
- `novanas_cloudbackup_last_success_timestamp` — updated per job.

## Gotchas

- WireGuard MTU: if the path has a lower MTU than 1500 (common on
  residential ISPs), drop the tunnel MTU to 1380 in the target spec.
- S3 endpoints with virtual-host-style addressing require a leading
  `.` in the endpoint; path-style is default and safer.
- Object Lock retention is per-object; the retention on the
  CloudBackupTarget is the *default* — older objects keep whatever
  retention they were stamped with at write time.
- First full sync can be enormous. Consider seeding via a portable
  disk (see `novanasctl replication seed --export / --import`).
