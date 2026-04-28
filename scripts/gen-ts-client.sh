#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../clients/typescript"
npm install --silent
npm run --silent gen
echo "Generated TypeScript client at clients/typescript/src/"
