#!/usr/bin/env python3
"""Perf regression comparator.

Compares a run's CSV output against the tracked baseline and fails
(exit 1) if any metric regresses beyond the threshold.

Two invocation styles are supported for backwards compatibility:

    # Positional: current baseline threshold
    python3 perf/compare.py <current.csv> <baseline.csv> <threshold>

    # Named (used by .github/workflows/perf-nightly.yml)
    python3 perf/compare.py \\
        --baseline perf/baseline.csv \\
        --results  perf/results/ \\
        --threshold-pct 10 \\
        --report   perf/results/report.md

In the named form --results may be a directory; all *.csv files are
concatenated. --threshold-pct accepts 10 (= 10%) or 0.10.

CSV schema (blank lines and `#` comments are ignored):

    test,mode,bs,iops,bandwidth_mbps,latency_ms_p99

Comparison rules:

* iops and bandwidth_mbps are "higher is better"; regression if
  current < baseline * (1 - threshold).
* latency_ms_p99 is "lower is better"; regression if
  current > baseline * (1 + threshold).
* Metrics with a baseline of 0 are skipped for that row (sentinel).
* Rows present in current but not in baseline are reported but not
  failed; rows in baseline but missing from current are failed
  (silent data loss is a regression too).
"""
from __future__ import annotations

import argparse
import csv
import io
import os
import sys
from pathlib import Path
from typing import Dict, Iterable, List, Tuple

KEY_COLS = ("test", "mode", "bs")
HIGHER_BETTER = ("iops", "bandwidth_mbps")
LOWER_BETTER = ("latency_ms_p99",)
METRIC_COLS = HIGHER_BETTER + LOWER_BETTER


def _strip_comments(raw: str) -> str:
    out = []
    for line in raw.splitlines():
        s = line.strip()
        if not s or s.startswith("#"):
            continue
        out.append(line)
    return "\n".join(out) + "\n"


def read_csv(path: Path) -> Dict[Tuple[str, str, str], Dict[str, float]]:
    """Return {(test, mode, bs): {metric: value}}."""
    text = _strip_comments(path.read_text())
    reader = csv.DictReader(io.StringIO(text))
    rows: Dict[Tuple[str, str, str], Dict[str, float]] = {}
    for row in reader:
        key = tuple(row.get(k, "").strip() for k in KEY_COLS)
        metrics: Dict[str, float] = {}
        for m in METRIC_COLS:
            v = row.get(m, "").strip()
            try:
                metrics[m] = float(v) if v else 0.0
            except ValueError:
                metrics[m] = 0.0
        rows[key] = metrics
    return rows


def read_csv_dir(path: Path) -> Dict[Tuple[str, str, str], Dict[str, float]]:
    if path.is_file():
        return read_csv(path)
    merged: Dict[Tuple[str, str, str], Dict[str, float]] = {}
    for p in sorted(path.glob("*.csv")):
        merged.update(read_csv(p))
    return merged


def normalize_threshold(t: float) -> float:
    """Accept 10 (percent) or 0.10 (fraction) and return a fraction."""
    if t > 1.0:
        return t / 100.0
    return t


def compare(
    current: Dict[Tuple[str, str, str], Dict[str, float]],
    baseline: Dict[Tuple[str, str, str], Dict[str, float]],
    threshold: float,
) -> Tuple[List[List[str]], bool]:
    """Return (table_rows, failed)."""
    thr = normalize_threshold(threshold)
    header = ["test", "mode", "bs", "metric", "baseline", "current", "delta_pct", "status"]
    rows: List[List[str]] = [header]
    failed = False

    # Baseline-first iteration so missing rows are caught.
    for key in sorted(baseline.keys()):
        base_m = baseline[key]
        cur_m = current.get(key)
        if cur_m is None:
            rows.append([*key, "*", "-", "-", "-", "MISSING"])
            failed = True
            continue
        for metric in METRIC_COLS:
            b = base_m.get(metric, 0.0)
            c = cur_m.get(metric, 0.0)
            if b == 0.0:
                continue  # sentinel: skip
            delta = (c - b) / b
            status = "ok"
            if metric in HIGHER_BETTER:
                if c < b * (1 - thr):
                    status = "REGRESSION"
                    failed = True
            else:  # latency
                if c > b * (1 + thr):
                    status = "REGRESSION"
                    failed = True
            rows.append([*key, metric, f"{b:g}", f"{c:g}", f"{delta*100:+.2f}%", status])

    # Extra rows (informational only).
    for key in sorted(set(current.keys()) - set(baseline.keys())):
        rows.append([*key, "*", "-", "-", "-", "NEW"])

    return rows, failed


def render_table(rows: List[List[str]]) -> str:
    widths = [max(len(str(r[i])) for r in rows) for i in range(len(rows[0]))]
    lines = []
    for i, r in enumerate(rows):
        lines.append("  ".join(str(c).ljust(widths[j]) for j, c in enumerate(r)))
        if i == 0:
            lines.append("  ".join("-" * w for w in widths))
    return "\n".join(lines)


def render_markdown(rows: List[List[str]], failed: bool, threshold: float) -> str:
    buf = io.StringIO()
    buf.write(f"# Perf regression report\n\n")
    buf.write(f"Threshold: {normalize_threshold(threshold)*100:.1f}%\n")
    buf.write(f"Status: **{'FAIL' if failed else 'OK'}**\n\n")
    buf.write("| " + " | ".join(rows[0]) + " |\n")
    buf.write("|" + "|".join("---" for _ in rows[0]) + "|\n")
    for r in rows[1:]:
        buf.write("| " + " | ".join(str(c) for c in r) + " |\n")
    return buf.getvalue()


def _parse_args(argv: List[str]) -> argparse.Namespace:
    # Positional-form shim: compare.py <current> <baseline> <threshold>
    if len(argv) == 3 and not any(a.startswith("-") for a in argv):
        ns = argparse.Namespace(
            current=argv[0],
            baseline=argv[1],
            threshold=float(argv[2]),
            report=None,
        )
        return ns
    p = argparse.ArgumentParser(description="Perf regression comparator")
    p.add_argument("--baseline", required=True, help="baseline CSV")
    p.add_argument(
        "--results",
        dest="current",
        required=True,
        help="current run CSV or directory of CSVs",
    )
    p.add_argument(
        "--threshold-pct",
        dest="threshold",
        type=float,
        default=10.0,
        help="regression threshold (percent or fraction)",
    )
    p.add_argument("--report", default=None, help="optional markdown report output")
    return p.parse_args(argv)


def main(argv: List[str] | None = None) -> int:
    ns = _parse_args(list(argv) if argv is not None else sys.argv[1:])
    current = read_csv_dir(Path(ns.current))
    baseline = read_csv(Path(ns.baseline))
    rows, failed = compare(current, baseline, ns.threshold)

    print(render_table(rows))
    if ns.report:
        Path(ns.report).parent.mkdir(parents=True, exist_ok=True)
        Path(ns.report).write_text(render_markdown(rows, failed, ns.threshold))
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
