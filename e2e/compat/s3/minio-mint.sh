#!/usr/bin/env bash
#
# minio-mint.sh — runs the MinIO mint S3 compatibility suite against the
# NovaNas s3gw. Any non-zero exit fails the job.
#
# Usage: minio-mint.sh [endpoint] [access_key] [secret_key]
# Defaults: https://localhost:9000 / novanas / novanas-secret
#
set -euo pipefail

ENDPOINT="${1:-${S3_ENDPOINT:-https://localhost:9000}}"
ACCESS="${2:-${S3_ACCESS_KEY:-novanas}}"
SECRET="${3:-${S3_SECRET_KEY:-novanas-secret}}"

ART="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/artifacts/mint"
mkdir -p "${ART}"

command -v docker >/dev/null || { echo "docker required" >&2; exit 2; }

# Parse host:port from endpoint so we can pass to mint (it takes server host).
SERVER="${ENDPOINT#*://}"
SCHEME="${ENDPOINT%%://*}"
ENABLE_HTTPS=0
[[ "${SCHEME}" == "https" ]] && ENABLE_HTTPS=1

echo "[mint] running minio/mint against ${ENDPOINT}"
docker run --rm --net=host \
  -e SERVER_ENDPOINT="${SERVER}" \
  -e ACCESS_KEY="${ACCESS}" \
  -e SECRET_KEY="${SECRET}" \
  -e ENABLE_HTTPS="${ENABLE_HTTPS}" \
  -e MINT_MODE="full" \
  -v "${ART}:/mint/log" \
  minio/mint:latest

echo "[mint] PASS — logs in ${ART}"
