#!/usr/bin/env bash
#
# fio-baseline.sh — run a fio matrix (rand/seq × 4k/64k/1M × read/write/mixed)
# against a NovaNas Dataset mounted at $MOUNT. Emits CSV to artifacts/fio.csv
# and fails if regression > $REGRESSION_PCT vs the previous run's baseline.
#
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ART="${HERE}/../artifacts/perf"
mkdir -p "${ART}"

MOUNT="${MOUNT:-/mnt/novanas-e2e}"
RUNTIME="${RUNTIME:-30}"
SIZE="${SIZE:-2G}"
REGRESSION_PCT="${REGRESSION_PCT:-10}"
BASELINE="${BASELINE:-${ART}/fio-baseline.csv}"
OUT="${ART}/fio.csv"

command -v fio >/dev/null || { echo "fio not installed" >&2; exit 2; }

if [[ ! -d "${MOUNT}" ]]; then
  echo "ERROR: ${MOUNT} not present; mount a Dataset there first" >&2
  exit 2
fi

echo "rw,bs,iops,bw_KBps,lat_us" > "${OUT}"

run_one() {
  local rw="$1" bs="$2"
  local job="fio-${rw}-${bs}"
  local tmp
  tmp=$(mktemp)
  fio --name="${job}" --directory="${MOUNT}" --size="${SIZE}" \
      --rw="${rw}" --bs="${bs}" --ioengine=libaio --direct=1 \
      --iodepth=32 --numjobs=4 --group_reporting \
      --runtime="${RUNTIME}" --time_based --output-format=json \
      --output="${tmp}" >/dev/null
  local iops bw lat
  iops=$(jq -r '.jobs[0].read.iops + .jobs[0].write.iops' "${tmp}")
  bw=$(jq -r '.jobs[0].read.bw + .jobs[0].write.bw' "${tmp}")
  lat=$(jq -r '((.jobs[0].read.clat_ns.mean // 0) + (.jobs[0].write.clat_ns.mean // 0)) / 1000' "${tmp}")
  echo "${rw},${bs},${iops},${bw},${lat}" >> "${OUT}"
  rm -f "${tmp}"
}

for rw in randread randwrite randrw read write; do
  for bs in 4k 64k 1M; do
    run_one "${rw}" "${bs}"
  done
done

echo "[fio-baseline] results written to ${OUT}"

# Regression check — only if a baseline exists.
if [[ -f "${BASELINE}" ]]; then
  echo "[fio-baseline] comparing against ${BASELINE} (tolerance ${REGRESSION_PCT}%)"
  python3 - "${BASELINE}" "${OUT}" "${REGRESSION_PCT}" <<'PY'
import csv, sys
baseline, current, tol_str = sys.argv[1:]
tol = float(tol_str) / 100.0
def load(p):
    m = {}
    with open(p) as f:
        r = csv.DictReader(f)
        for row in r:
            m[(row["rw"], row["bs"])] = float(row["iops"])
    return m
b = load(baseline); c = load(current)
regressions = []
for k, base in b.items():
    cur = c.get(k, 0.0)
    if base > 0 and (base - cur) / base > tol:
        regressions.append((k, base, cur))
for k, base, cur in regressions:
    print(f"REGRESSION {k}: baseline={base:.0f} current={cur:.0f}")
sys.exit(1 if regressions else 0)
PY
fi

echo "[fio-baseline] PASS"
