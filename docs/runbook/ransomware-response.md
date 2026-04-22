# Ransomware response

Steps to take when ransomware is suspected or confirmed on systems that
use NovaNas for storage. The goal is to preserve evidence, prevent the
attacker from overwriting backups, and restore clean data with the
lowest possible RPO.

## Decide fast

If any of the following are true, treat this as a ransomware incident
and start at step 1 below:

- Users report files renamed with unknown extensions (`.locked`,
  `.encrypted`, random 8-char suffix, etc.).
- Snapshots are disappearing faster than the retention policy allows.
- A ransom note has appeared on a share.
- Unusual outbound network traffic from a client that mounts NovaNas.

Do not reboot, do not delete anything, do not "just try restoring" —
you will lose evidence and may overwrite an immutable snapshot.

## 1. Freeze writes

Cut off client access to every affected share immediately:

```sh
# Put the affected SMB/NFS servers into paused state (reads blocked, no writes).
novanasctl share pause <share> --reason ransomware-incident
novanasctl share pause --all-in-namespace <ns>
```

If uncertain which shares are affected, pause all of them. Cheaper to
re-open than to let encryption continue.

## 2. Protect the backups

NovaNas snapshots and cloud-backup targets should already be immutable
(Object Lock in compliance mode). Verify:

```sh
novanasctl snapshot list <dataset> | head
# Expect: "immutable=true, lock-until=<future date>"

novanasctl cloudbackup target get <target> -o json \
  | jq '.spec.objectLock, .status.lockedObjects'
# objectLock should be "compliance" or "governance"; lockedObjects > 0.
```

If any backup is not locked, suspend the replication schedule so the
attacker cannot overwrite the remote copy with encrypted data:

```sh
novanasctl replication job pause <job>
```

## 3. Preserve evidence

Snapshot the current state before anything moves:

```sh
novanasctl snapshot create <dataset> \
  --name incident-$(date +%FT%H%M) \
  --immutable --lock-days 90
```

Capture live logs:

```sh
mkdir -p /tmp/ir-$(date +%F)
journalctl --since "48 hours ago" > /tmp/ir-*/journal.txt
novanasctl audit export --since 48h -o /tmp/ir-*/audit.jsonl
```

Note (in a separate offline document):
- First reported time.
- Clients involved (IPs, users).
- Extensions observed on encrypted files.
- Ransom note contents (verbatim).

## 4. Identify the earliest clean snapshot

```sh
novanasctl snapshot list <dataset> --sort=timestamp
```

Walk snapshots from newest to oldest; for each, mount read-only and
check a known-good file:

```sh
novanasctl snapshot mount <dataset>@<snap> /mnt/probe --read-only
sha256sum /mnt/probe/<known-file>    # compare against a pre-incident hash
novanasctl snapshot unmount <dataset>@<snap>
```

The newest snapshot where the hash matches is your recovery point.

## 5. Restore

Roll the dataset back:

```sh
# Full rollback — destroys newer snapshots on this dataset.
novanasctl dataset rollback <dataset> --to <snap> --confirm

# OR clone into a new dataset for side-by-side inspection first:
novanasctl snapshot clone <dataset>@<snap> --into <dataset>-clean
```

If the on-prem copy was also compromised, restore from cloud backup:

```sh
novanasctl cloudbackup restore \
  --target <target> \
  --snapshot <timestamp> \
  --into <dataset>-restored
```

## 6. Harden before re-opening

Before un-pausing the shares:

- Patch or quarantine the originally infected clients.
- Rotate all SMB/NFS user passwords and API tokens.
- Verify Object Lock is enabled on all remaining unprotected targets.
- Shorten snapshot intervals for the next 30 days (e.g., hourly).
- Enable `AuditPolicy` on affected datasets at `verbose` level.

Then:

```sh
novanasctl share resume <share>
```

## Post-incident

- File a written incident report referencing the immutable
  `incident-*` snapshot created in step 3.
- Keep the incident snapshot for at least 90 days (legal hold).
- Schedule a blameless review — what gave the attacker write access?

## Gotchas

- If Object Lock is in *governance* mode, an insider with the bypass
  role can still delete locked objects. Only *compliance* mode is
  truly immune.
- Pausing shares does not interrupt in-flight writes that are already
  committed to the dataplane; a window of seconds of encryption may
  still land. That is why step 3 runs before step 4.
- Rolling back a dataset destroys newer snapshots. If you are not sure
  which snapshot is clean, clone — don't roll back.
- `cloudbackup restore` bills egress on the target provider. Budget for
  this in your DR plan.
