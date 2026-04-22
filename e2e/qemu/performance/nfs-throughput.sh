#!/usr/bin/env bash
#
# nfs-throughput.sh — large-file streaming throughput over NFSv4 against a
# NovaNas Share. Emits a CSV row to artifacts/perf/nfs.csv.
#
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ART="${HERE}/../artifacts/perf"
mkdir -p "${ART}"

NFS_SERVER="${NFS_SERVER:-localhost}"
NFS_EXPORT="${NFS_EXPORT:-/export/e2e-share-photos}"
MOUNT="${MOUNT:-/mnt/novanas-nfs-e2e}"
SIZE_MB="${SIZE_MB:-4096}"
OUT="${ART}/nfs.csv"

command -v mount.nfs4 >/dev/null || { echo "nfs-common not installed" >&2; exit 2; }
command -v dd >/dev/null || { echo "dd missing" >&2; exit 2; }

sudo mkdir -p "${MOUNT}"
sudo mount -t nfs4 -o vers=4.2 "${NFS_SERVER}:${NFS_EXPORT}" "${MOUNT}"
trap 'sudo umount -f "${MOUNT}" || true' EXIT

TMP="${MOUNT}/e2e-bigfile.$$"
echo "[nfs-throughput] write ${SIZE_MB} MiB"
WRITE=$( { time dd if=/dev/urandom of="${TMP}" bs=1M count="${SIZE_MB}" oflag=direct conv=fsync 2>/dev/null; } 2>&1 | awk '/real/ {print $2}' )
echo "[nfs-throughput] read  ${SIZE_MB} MiB"
READ=$(  { time dd if="${TMP}" of=/dev/null bs=1M iflag=direct 2>/dev/null; } 2>&1 | awk '/real/ {print $2}' )
rm -f "${TMP}"

if [[ ! -f "${OUT}" ]]; then echo "size_mb,write_time,read_time" > "${OUT}"; fi
echo "${SIZE_MB},${WRITE},${READ}" >> "${OUT}"
echo "[nfs-throughput] PASS (write=${WRITE} read=${READ})"
