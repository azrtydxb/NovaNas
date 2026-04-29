#!/bin/bash
# Install KubeVirt operator + CR. Waits for Deployed phase.
set -euo pipefail
KV=${KV:-v1.4.0}
sudo k3s kubectl create -f https://github.com/kubevirt/kubevirt/releases/download/$KV/kubevirt-operator.yaml
sudo k3s kubectl create -f https://github.com/kubevirt/kubevirt/releases/download/$KV/kubevirt-cr.yaml
echo "Waiting for KubeVirt Deployed..."
until sudo k3s kubectl -n kubevirt get kv kubevirt -o jsonpath="{.status.phase}" 2>/dev/null | grep -q Deployed; do sleep 5; done
echo "KubeVirt $KV ready."
