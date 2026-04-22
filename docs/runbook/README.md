# NovaNas production runbook

Short, task-oriented guides for operators running NovaNas in production.
Each page is a step-by-step procedure with copy-paste commands, not
reference material. Skim the index, find the scenario you are in, then
follow the steps in order.

If a procedure here contradicts the CRD reference or architecture docs,
the referenced doc wins — file a PR against this runbook.

## Index

| Scenario | Page |
| --- | --- |
| Adding disks, sizing a new pool, picking a tier | [hardware-expansion.md](hardware-expansion.md) |
| SMART warnings, draining, hot-swap, wipe | [disk-replacement.md](disk-replacement.md) |
| Setting up off-site replication targets | [offsite-replication.md](offsite-replication.md) |
| Applying an OS update, rolling back | [os-upgrade.md](os-upgrade.md) |
| Responding to a ransomware incident | [ransomware-response.md](ransomware-response.md) |
| Full-box failure, restoring on new hardware | [disaster-recovery.md](disaster-recovery.md) |

## Conventions

- Commands are shown with the `$` prefix for shell, `>` for SQL/psql,
  and `kubectl` without a prefix.
- Placeholders are angle-bracketed: `<pool-name>`, `<node>`.
- Assume the operator is logged in to `novanasctl` and has `admin` role.
- All kubectl commands default to the `novanas-system` namespace unless
  explicitly stated.

## Before you start

1. Check the dashboard for active alerts — if something upstream is
   already failing, don't pile on.
2. Snapshot the relevant dataset/pool first if the procedure is
   destructive. `novanasctl snapshot create <dataset> --name pre-<op>`.
3. Announce maintenance in `#ops` (or your equivalent) so on-call knows.

## Related

- [Troubleshooting guide](../troubleshooting/README.md) — diagnostics when
  something is actively broken.
- [CRD reference](../05-crd-reference.md) — authoritative resource shape.
- [Decision log](../14-decision-log.md) — why things are the way they are.
