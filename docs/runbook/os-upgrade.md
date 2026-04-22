# OS upgrade (RAUC)

NovaNas ships an A/B root filesystem managed by RAUC. This page covers
applying an update bundle, rolling back if it boots but behaves badly,
and the usual gotchas.

## How updates land

1. Operator publishes a signed `.raucb` bundle to the update channel.
2. `UpdatePolicy` controller polls the channel on its schedule.
3. When a new bundle is available and the policy window is open, the
   bundle is downloaded to `/var/lib/rauc/`.
4. RAUC installs it into the **inactive** slot (A or B, whichever is not
   running).
5. The inactive slot is marked `try`. A reboot boots into the new slot.
6. If the boot succeeds and the system is healthy for the probation
   window (default 10 min), RAUC marks the slot `good`.
7. If probation fails, the bootloader automatically returns to the old
   slot on the next reboot.

Read the architecture doc for the full state machine:
[`docs/06-boot-install-update.md`](../06-boot-install-update.md).

## Pre-flight

- Check the current slot and update status:

  ```sh
  rauc status
  novanasctl system update status
  ```

- Confirm both slots are bootable:

  ```sh
  rauc status | grep -E 'state|boot status'
  # Each slot should read state=good or state=active; no "bad".
  ```

- Drain replicated sessions if you cannot tolerate a brief window of
  NFS/SMB hiccups around the reboot:

  ```sh
  novanasctl system maintenance enter --message "OS upgrade in 5 min"
  ```

## Apply an update

1. Trigger an install (usually the controller does this automatically;
   this is the manual path):

   ```sh
   novanasctl system update install --bundle <url-or-channel>
   ```

2. Watch the install log:

   ```sh
   journalctl -u rauc -f
   ```

3. Reboot when prompted:

   ```sh
   systemctl reboot
   ```

4. After reboot verify the new version:

   ```sh
   novanasctl system info
   rauc status
   ```

5. Exit maintenance mode:

   ```sh
   novanasctl system maintenance exit
   ```

## Rollback

### Automatic (boot failure)

If the new slot fails to boot or crashes during probation, the
bootloader flips back automatically. No action needed. Investigate via:

```sh
journalctl -b -1 -xe       # previous boot log
```

### Manual (the new slot boots, but behaves badly)

While still in the probation window:

```sh
rauc status mark-bad        # marks the current slot bad
systemctl reboot            # boots the previous slot
```

After the probation window has closed (slot was marked `good`):

```sh
rauc status mark-active other   # promote the old slot
systemctl reboot
```

Then verify you are back on the older version and file a bug.

## Common gotchas

- **Probation too short for your workload.** If your NFS clients take
  > 10 min to reconnect, bump `UpdatePolicy.spec.probation` to 30 min.
  The controller will not mark the slot `good` until it passes.
- **Signature mismatch.** If `rauc install` fails with `no valid
  signatures`, the bundle is signed by a cert not in
  `/etc/rauc/ca.pem`. Do *not* bypass — confirm the bundle origin first.
- **Out of space on /data.** The bundle is downloaded under
  `/var/lib/rauc/`. If the pool backing /data is > 95% full the install
  aborts. Free space first.
- **Updates disabled mid-install.** If an operator sets
  `UpdatePolicy.spec.enabled=false` while a download is in progress,
  the current run completes but no new runs start. The slot stays in
  `try` state until an operator manually marks it `good` or `bad`.
- **NovaEdge / NovaNet on separate cycles.** Those components run on
  Kubernetes and are not part of the RAUC slot. They roll forward on
  pod restart. Check their versions after the OS update completes.

## Verification checklist

- [ ] `rauc status` shows the new slot as `active, good`.
- [ ] `novanasctl system info` reports the expected version.
- [ ] NFS/SMB clients reconnected (no open alerts).
- [ ] Pool health, replication lag, snapshot schedules all green.
- [ ] Release notes read, any post-upgrade manual steps executed.
