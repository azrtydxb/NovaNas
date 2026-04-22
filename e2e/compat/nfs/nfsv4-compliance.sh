#!/usr/bin/env bash
#
# nfsv4-compliance.sh — run an NFSv4 conformance suite against a NovaNas Share.
# Preferred: pynfs (Python, upstream at linux-nfs.org). Falls back to the
# connectathon-04 (cthon04) basic+general+special+lock suites if pynfs is not
# available.
#
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ART="${HERE}/../../artifacts/nfs"
mkdir -p "${ART}"

NFS_SERVER="${NFS_SERVER:-localhost}"
NFS_EXPORT="${NFS_EXPORT:-/export/e2e-share-photos}"
SUITE="${SUITE:-auto}"   # one of: auto | pynfs | cthon04

PYNFS_REPO="${PYNFS_REPO:-git://linux-nfs.org/~bfields/pynfs.git}"
CTHON_REPO="${CTHON_REPO:-https://github.com/linux-test-project/cthon04.git}"
CACHE="${HERE}/.cache"
mkdir -p "${CACHE}"

run_pynfs() {
  if [[ ! -d "${CACHE}/pynfs" ]]; then
    git clone --depth 1 "${PYNFS_REPO}" "${CACHE}/pynfs"
  fi
  cd "${CACHE}/pynfs/nfs4.1"
  python3 testserver.py "${NFS_SERVER}:${NFS_EXPORT}" --maketree all \
    --outfile "${ART}/pynfs.log"
}

run_cthon04() {
  if [[ ! -d "${CACHE}/cthon04" ]]; then
    git clone --depth 1 "${CTHON_REPO}" "${CACHE}/cthon04"
    make -C "${CACHE}/cthon04"
  fi
  cd "${CACHE}/cthon04"
  sudo ./server -o "vers=4.2" -p "${NFS_EXPORT}" -m /mnt/cthon-e2e "${NFS_SERVER}" \
    | tee "${ART}/cthon04.log"
}

case "${SUITE}" in
  pynfs)   run_pynfs ;;
  cthon04) run_cthon04 ;;
  auto)
    if command -v python3 >/dev/null; then run_pynfs; else run_cthon04; fi ;;
  *) echo "unknown SUITE=${SUITE}" >&2; exit 2 ;;
esac

echo "[nfsv4-compliance] PASS — logs in ${ART}"
