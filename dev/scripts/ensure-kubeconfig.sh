#!/usr/bin/env bash
# Make sure ~/.kube/novanas-dev.kubeconfig exists before `docker compose up`
# binds it into the api container. If the kind cluster isn't running we
# still need a file so the bind mount doesn't fail; a tiny placeholder is
# written that tells the API there is no cluster.

set -euo pipefail

TARGET="${HOME}/.kube/novanas-dev.kubeconfig"
mkdir -p "$(dirname "${TARGET}")"

if [ -f "${TARGET}" ]; then
  exit 0
fi

cat > "${TARGET}" <<'EOF'
# Placeholder kubeconfig — no kind cluster is running.
# Run `make dev-cluster-up` to replace this with a real one.
apiVersion: v1
kind: Config
clusters: []
contexts: []
users: []
EOF

echo "Wrote placeholder kubeconfig at ${TARGET}"
