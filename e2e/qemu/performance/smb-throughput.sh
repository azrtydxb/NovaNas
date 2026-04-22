#!/usr/bin/env bash
#
# smb-throughput.sh — large-file streaming over SMB3 against a NovaNas Share.
# Emits artifacts/perf/smb.csv.
#
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ART="${HERE}/../artifacts/perf"
mkdir -p "${ART}"

SMB_SERVER="${SMB_SERVER:-localhost}"
SHARE="${SHARE:-e2e-share-photos}"
SMB_USER="${SMB_USER:-e2e}"
SMB_PASS="${SMB_PASS:-e2e-password}"
MOUNT="${MOUNT:-/mnt/novanas-smb-e2e}"
SIZE_MB="${SIZE_MB:-4096}"
OUT="${ART}/smb.csv"

command -v mount.cifs >/dev/null || { echo "cifs-utils not installed" >&2; exit 2; }

sudo mkdir -p "${MOUNT}"
sudo mount -t cifs "//${SMB_SERVER}/${SHARE}" "${MOUNT}" \
  -o "username=${SMB_USER},password=${SMB_PASS},vers=3.1.1,cache=none"
trap 'sudo umount -f "${MOUNT}" || true' EXIT

TMP="${MOUNT}/e2e-bigfile.$$"
echo "[smb-throughput] write ${SIZE_MB} MiB"
WRITE=$( { time dd if=/dev/urandom of="${TMP}" bs=1M count="${SIZE_MB}" 2>/dev/null; } 2>&1 | awk '/real/ {print $2}' )
echo "[smb-throughput] read  ${SIZE_MB} MiB"
READ=$(  { time dd if="${TMP}" of=/dev/null bs=1M 2>/dev/null; } 2>&1 | awk '/real/ {print $2}' )
rm -f "${TMP}"

if [[ ! -f "${OUT}" ]]; then echo "size_mb,write_time,read_time" > "${OUT}"; fi
echo "${SIZE_MB},${WRITE},${READ}" >> "${OUT}"
echo "[smb-throughput] PASS (write=${WRITE} read=${READ})"
