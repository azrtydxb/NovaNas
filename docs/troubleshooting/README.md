# NovaNas troubleshooting guide

Diagnostic-first reference for when something is broken. Each page is
organised around symptoms: "the user sees X; run these commands; if
this, then that". Use this when you need to *diagnose*. Use
[`../runbook/`](../runbook/) when you need to *execute a procedure*.

## Top 10 known issues

Quick lookup for the most common pain points. Each entry links to the
full diagnostic walkthrough.

1. **SMB share fails to mount with `NT_STATUS_LOGON_FAILURE`** —
   usually a Kerberos ticket that expired; see
   [identity.md](identity.md#smb-nt_status_logon_failure).
2. **Pool rebuild stuck at 99%** — a single unreadable chunk on a
   drained disk; see [storage.md](storage.md#rebuild-stuck-at-99).
3. **Scrub reports checksum errors** — bad RAM, bad cable, or bad
   drive; see [storage.md](storage.md#scrub-checksum-errors).
4. **fio baseline is 30% below spec** — CPU C-states or NUMA
   misalignment; see [performance.md](performance.md#fio-below-baseline).
5. **Keycloak login returns "Invalid credentials" for correct
   password** — user federation sync stale; see
   [identity.md](identity.md#federation-sync-failure).
6. **OpenBao pods in `CrashLoopBackOff` after reboot** — seal keys not
   present; see [identity.md](identity.md#openbao-unseal).
7. **NovaEdge VIP does not float after a node loss** — BGP session
   with upstream router did not come back; see
   [networking.md](networking.md#novaedge-vip-stuck).
8. **NFS mount hangs on client** — server-side grace period; see
   [networking.md](networking.md#nfs-hang).
9. **Replication job stays "Running" forever** — snapshot pruning
   race; see [storage.md](storage.md#replication-stuck).
10. **API returns 401 for a valid token** — clock skew between
    NovaNas nodes and the client; see
    [identity.md](identity.md#token-clock-skew).

## Topic pages

| Topic | Page |
| --- | --- |
| Chunk engine, rebuilds, scrubs, WAL | [storage.md](storage.md) |
| VIPs, NovaNet policies, DNS, mDNS | [networking.md](networking.md) |
| Keycloak, OpenBao, federation, tokens | [identity.md](identity.md) |
| fio, NFS/SMB throughput, latency | [performance.md](performance.md) |

## How to use this guide

Each symptom entry has four parts:

1. **Symptom** — what the user sees.
2. **Diagnose** — commands to run, what to look for in output.
3. **Root causes** — ordered most → least likely.
4. **Remediate** — the fix, or a link to the appropriate runbook.

If a symptom is not listed:

1. Check the dashboard for unusual metrics in the affected subsystem.
2. Grep the controller logs:

   ```sh
   kubectl -n novanas-system logs -l app=novanas-operators --tail=500 \
     | grep -i <subsystem>
   ```

3. Check `kubectl events -A --sort-by=.lastTimestamp` for Warning
   events from the last hour.
4. If still stuck, file an issue with the `triage` label and attach
   `novanasctl support bundle` output.
