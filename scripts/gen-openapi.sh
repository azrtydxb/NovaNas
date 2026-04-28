#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p internal/api/oapi
oapi-codegen -generate types -package oapi -o internal/api/oapi/types.go api/openapi.yaml
echo "Generated internal/api/oapi/types.go"
