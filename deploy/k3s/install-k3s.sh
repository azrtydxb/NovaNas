#!/bin/bash
# Install k3s with Traefik and ServiceLB disabled.
set -euo pipefail
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable=traefik --disable=servicelb --write-kubeconfig-mode=644 --node-name=$(hostname) --kube-controller-manager-arg=allocate-node-cidrs=false --flannel-backend=host-gw" sh -
echo "k3s installed. Use 'sudo k3s kubectl' or copy /etc/rancher/k3s/k3s.yaml."
