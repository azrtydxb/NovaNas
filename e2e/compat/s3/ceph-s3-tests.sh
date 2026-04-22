#!/usr/bin/env bash
#
# ceph-s3-tests.sh — runs the upstream Ceph s3-tests suite against NovaNas
# s3gw. Clones the repo on first run, writes s3tests.conf from env, then runs
# nosetests with our allowlist.
#
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECKOUT="${HERE}/.cache/s3-tests"
REPO="${CEPH_S3_TESTS_REPO:-https://github.com/ceph/s3-tests.git}"
REF="${CEPH_S3_TESTS_REF:-master}"

ENDPOINT="${1:-${S3_ENDPOINT:-https://localhost:9000}}"
ACCESS="${2:-${S3_ACCESS_KEY:-novanas}}"
SECRET="${3:-${S3_SECRET_KEY:-novanas-secret}}"

HOST="${ENDPOINT#*://}"; HOST="${HOST%%/*}"
PORT="${HOST##*:}"; [[ "${PORT}" == "${HOST}" ]] && PORT=443
HOSTNAME="${HOST%%:*}"
SCHEME="${ENDPOINT%%://*}"
IS_SECURE="False"; [[ "${SCHEME}" == "https" ]] && IS_SECURE="True"

if [[ ! -d "${CHECKOUT}" ]]; then
  git clone --depth 1 --branch "${REF}" "${REPO}" "${CHECKOUT}"
fi
cd "${CHECKOUT}"

python3 -m venv .venv
# shellcheck disable=SC1091
. .venv/bin/activate
pip install --quiet -r requirements.txt

cat > s3tests.conf <<EOF
[DEFAULT]
host = ${HOSTNAME}
port = ${PORT}
is_secure = ${IS_SECURE}

[fixtures]
bucket prefix = e2e-s3tests-{random}-

[s3 main]
display_name = E2E Main
user_id = e2e-main
email = e2e-main@novanas.local
api_name = default
access_key = ${ACCESS}
secret_key = ${SECRET}

[s3 alt]
display_name = E2E Alt
user_id = e2e-alt
email = e2e-alt@novanas.local
access_key = ${ACCESS}
secret_key = ${SECRET}

[s3 tenant]
display_name = E2E Tenant
user_id = e2e-tenant
email = e2e-tenant@novanas.local
access_key = ${ACCESS}
secret_key = ${SECRET}
tenant = e2e
EOF

export S3TEST_CONF="${PWD}/s3tests.conf"

# NovaNas s3gw-supported subset: bucket, object, multipart, lifecycle, tagging.
# Skips: IAM/STS, BucketPolicy (until RBAC integration lands), Torrent.
pytest -q \
  -m 'not fails_on_rgw and not lifecycle_expiration and not sse_s3' \
  s3tests_boto3/functional/test_s3.py
echo "[ceph-s3-tests] PASS"
