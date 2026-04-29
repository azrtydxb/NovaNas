# NovaNAS CSI driver — install guide

This directory packages the `csi.novanas.io` driver for a single-node
k3s cluster running on the same machine as the NovaNAS storage host.

## Layout

```
deploy/csi/
  Dockerfile                container image (multi-stage, debian-slim runtime)
  build-image.sh            docker build + optional k3s containerd import
  install.sh                kubectl apply in dependency order, wait for Ready
  uninstall.sh              reverse, with orphan-PV warning
  manifests/
    00-namespace.yaml
    10-rbac.yaml            ServiceAccounts, ClusterRoles, leader-election lease
    20-csidriver.yaml       CSIDriver object
    30-volumesnapshotclass.yaml
    40-storageclass-fs.yaml      ZFS dataset (default class)
    40-storageclass-block.yaml   ZFS zvol
    50-controller.yaml      Deployment with sidecars (provisioner/resizer/snapshotter)
    60-node.yaml            DaemonSet with node-driver-registrar
    70-secret-template.yaml documentation only — do NOT apply
```

## Prerequisites

- `nova-api` reachable from the host on `https://127.0.0.1:8444` (default
  for the systemd unit at `deploy/systemd/nova-api.service`).
- k3s 1.28+ with the snapshot CRDs installed:
  ```
  kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.1.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
  kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.1.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml
  kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.1.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml
  ```
  Plus the snapshot-controller Deployment from the same release.
- ZFS pool named `tank` with parent dataset `tank/csi` (created by
  the operator; the driver does NOT manage the pool).
- A Keycloak realm with a `nova-operator` role; the CSI driver uses a
  service-account JWT carrying that role.
- The local CA PEM that signed the `nova-api` TLS cert.

## Step-by-step

### 1. Build the image

```
IMPORT_K3S=1 ./deploy/csi/build-image.sh
# or: make csi-image
```

`IMPORT_K3S=1` pipes the image into k3s's containerd so no registry is
required. Without it, push to a registry your cluster can pull from and
edit `image:` in the manifests.

### 2. Create the auth Secret

The Secret must contain a JWT and the nova-api CA. Don't commit either.

```
JWT=$(curl -sf "$KEYCLOAK_TOKEN_URL" \
  -d grant_type=client_credentials \
  -d client_id=nova-csi \
  -d client_secret="$NOVA_CSI_CLIENT_SECRET" \
  | jq -r .access_token)

kubectl create namespace nova-csi --dry-run=client -o yaml | kubectl apply -f -
kubectl -n nova-csi create secret generic nova-csi-auth \
    --from-literal=token="${JWT}" \
    --from-file=ca.crt=/etc/nova-ca/ca.crt
```

Token rotation: re-create the Secret in place, then
`kubectl -n nova-csi rollout restart deployment/nova-csi-controller daemonset/nova-csi-node`.

### 3. Install

```
./deploy/csi/install.sh
# or: make csi-deploy
```

The script applies manifests in order, waits for the controller and node
to be Ready, and prints the resulting pods.

### 4. Smoke test

```
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nova-csi-test
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
  # storageClassName omitted -> uses novanas-fs (default)
EOF

kubectl get pvc nova-csi-test -w
```

The PVC stays `Pending` until a Pod consumes it (because of
`volumeBindingMode: WaitForFirstConsumer`). Bind a busybox Pod with the
PVC and watch the dataset appear under `tank/csi/`.

## StorageClass parameters

| StorageClass    | Param         | Default     | Meaning |
|-----------------|---------------|-------------|---------|
| `novanas-fs`    | `pool`        | `tank`      | ZFS pool to provision into |
|                 | `parent`      | `tank/csi`  | Parent dataset under which CSI volumes are created |
|                 | `compression` | `lz4`       | Set on the new dataset |
|                 | `recordsize`  | `16K`       | Set on the new dataset |
| `novanas-block` | `pool`        | `tank`      | Same |
|                 | `parent`      | `tank/csi`  | Same |
|                 | `compression` | `lz4`       | Set on the new zvol |
|                 | `volblocksize`| `16K`       | Set on the new zvol |
|                 | `fsType`      | `ext4`      | Used when consumer requests `volumeMode: Filesystem` |

## Troubleshooting

### Controller logs

```
kubectl -n nova-csi logs deploy/nova-csi-controller -c nova-csi
kubectl -n nova-csi logs deploy/nova-csi-controller -c csi-provisioner
kubectl -n nova-csi logs deploy/nova-csi-controller -c csi-resizer
kubectl -n nova-csi logs deploy/nova-csi-controller -c csi-snapshotter
```

### Node logs

```
kubectl -n nova-csi logs ds/nova-csi-node -c nova-csi
kubectl -n nova-csi logs ds/nova-csi-node -c csi-node-driver-registrar
```

### CrashLoopBackOff with "no token provided"

Secret missing or wrong shape. Check
`kubectl -n nova-csi get secret nova-csi-auth -o yaml` and confirm the
`token` and `ca.crt` keys exist and are base64-encoded.

### "x509: certificate signed by unknown authority"

The `ca.crt` in the Secret doesn't match the CA that signed the nova-api
cert. Re-export the CA on the host: `cat /etc/nova-ca/ca.crt`.

### Stuck PV after PVC deletion

If the underlying dataset can't be destroyed (e.g. busy mount), the PV
sticks in `Terminating`. From the host:

```
sudo zfs list -t all | grep tank/csi
sudo umount /var/lib/kubelet/pods/*/volumes/kubernetes.io~csi/<pv-name>/mount || true
sudo zfs destroy tank/csi/<pv-name>
kubectl patch pv <pv-name> -p '{"metadata":{"finalizers":null}}'
```

This is the same recovery pattern as any external CSI driver; do it
deliberately because the dataset may hold real data.

### Reach nova-api from inside the controller pod

```
kubectl -n nova-csi exec deploy/nova-csi-controller -c nova-csi -- \
    sh -c 'curl -k --cacert /etc/nova-csi/ca.crt -H "Authorization: Bearer $(cat /etc/nova-csi/token)" https://127.0.0.1:8444/healthz'
```

If this fails, the systemd nova-api isn't listening on 127.0.0.1:8444
or the pod isn't actually using `hostNetwork: true`.
