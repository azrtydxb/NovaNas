# NovaNas perf baselines

This directory feeds the two perf-regression workflows:

- `.github/workflows/perf-nightly.yml` — weekly QEMU-based perf suite.
  Runs fio / NFS / SMB against the full stack and compares the result
  with `baseline.csv`.
- `.github/workflows/perf-gate.yml` — PR gate that requires a
  `perf-regression-ok` label whenever `storage/`, `packages/dataplane/`,
  or `perf/` is touched.

## Files

| File | Purpose |
|------|---------|
| `baseline.csv` | Tracked per-test expected metrics. |
| `compare.py`   | Comparator that diffs a run against the baseline. |

## `baseline.csv` schema

```
test,mode,bs,iops,bandwidth_mbps,latency_ms_p99
```

* `test` — `fio`, `nfs`, `smb`, …
* `mode` — `seqread`, `seqwrite`, `randread`, `randwrite`, `read`, `write`
* `bs` — block size (e.g. `4k`, `64k`, `1m`)
* `iops` / `bandwidth_mbps` — higher is better; `0` = metric not
  applicable for this row.
* `latency_ms_p99` — lower is better.

Lines beginning with `#` are comments.

## `compare.py` usage

Two invocation styles are supported.

### Positional (simple)

```sh
python3 perf/compare.py <current.csv> <baseline.csv> <threshold>
```

`threshold` is either a fraction (`0.10`) or a percentage (`10`).
Exit `0` if within tolerance, `1` if any metric regresses.

### Named (used by workflows)

```sh
python3 perf/compare.py \
  --baseline perf/baseline.csv \
  --results  perf/results/ \
  --threshold-pct 10 \
  --report   perf/results/report.md
```

`--results` may be either a single CSV or a directory of CSVs (all
`*.csv` are merged). `--report` writes a markdown diff table that the
nightly workflow surfaces via GitHub issues on failure.

## Re-capturing baselines

The numbers in `baseline.csv` are intentionally conservative seeds.
When a legitimate improvement or planned regression changes the
expected values:

1. Run the nightly workflow on your branch via `workflow_dispatch`.
2. Download the `perf-results` artifact; merge its CSVs into a new
   `baseline.csv` (keep the comment header).
3. Open a PR with only the baseline change.
4. Apply the `perf-regression-ok` label to satisfy `perf-gate.yml`.
5. After merge the nightly uses the new numbers.

## Adding a new test row

1. Emit a CSV with the same schema from the test script (see
   `e2e/qemu/performance/*.sh`).
2. Add a matching row to `baseline.csv` with realistic initial
   numbers.
3. Comment explaining units if the test is unusual.
