# k3s + NovaNAS CSI + KubeVirt deployment notes

## Boot order
postgresql → openbao → keycloak → nova-api → k3s

## k3s install
```bash
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable=traefik --disable=servicelb --write-kubeconfig-mode=644 --node-name=$(hostname) --kube-controller-manager-arg=allocate-node-cidrs=false --flannel-backend=host-gw" sh -
```

## Storage pool
```bash
zpool create -f -o ashift=12 tank raidz2 <hdds...> log <slog-nvme> cache <l2arc-ssds>
zfs create -o compression=lz4 -o atime=off -o recordsize=16K tank/csi
```

## CSI driver
See `/deploy/csi/`. Build image, import into containerd, create Secret with Keycloak JWT, apply manifests.

## KubeVirt
```bash
kubectl create -f https://github.com/kubevirt/kubevirt/releases/download/v1.4.0/kubevirt-operator.yaml
kubectl create -f https://github.com/kubevirt/kubevirt/releases/download/v1.4.0/kubevirt-cr.yaml
```

## Tested: VM with NovaNAS-backed disk
```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: test-vm
spec:
  runStrategy: Always
  template:
    spec:
      domain:
        cpu: { cores: 1 }
        memory: { guest: 256Mi }
        devices:
          disks: [{ name: rootdisk, disk: { bus: virtio } }, { name: zvol-data, disk: { bus: virtio } }]
          interfaces: [{ name: default, masquerade: {} }]
      networks: [{ name: default, pod: {} }]
      volumes:
      - name: rootdisk
        containerDisk: { image: quay.io/kubevirt/cirros-container-disk-demo:latest }
      - name: zvol-data
        persistentVolumeClaim: { claimName: vm-data }
```
