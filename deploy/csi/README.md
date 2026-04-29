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

### 2. Create the Keycloak client and auth Secret

NovaNAS CSI uses the OAuth2 **client_credentials** grant. The driver fetches
a fresh access token at startup and refreshes it in the background at ~70%
of each token's remaining lifetime, so the static-JWT-in-a-Secret pattern is
gone — operators only ship the client secret.

#### 2a. Create the `nova-csi` client in Keycloak

A helper that runs `kcadm.sh` against your Keycloak instance:

```
KC_URL=https://kc.example:8443 \
KC_ADMIN_USER=admin \
KC_ADMIN_PASS='...' \
  ./deploy/keycloak/create-csi-client.sh > /tmp/csi-client.json
```

The script is idempotent: it creates `nova-csi` as a confidential
service-account client (`clientAuthenticatorType=client-secret`,
`serviceAccountsEnabled=true`, both standard and direct-access flows
disabled), maps the `nova-operator` realm role to its service account,
and rotates the client secret. It prints
`{"clientId":"nova-csi","clientSecret":"..."}` to stdout.

#### 2b. Create the Kubernetes Secret

```
SECRET=$(jq -r .clientSecret /tmp/csi-client.json)

kubectl create namespace nova-csi --dry-run=client -o yaml | kubectl apply -f -
kubectl -n nova-csi create secret generic nova-csi-auth \
    --from-literal=oidc-client-id=nova-csi \
    --from-literal=oidc-client-secret="${SECRET}" \
    --from-file=ca.crt=/etc/nova-ca/ca.crt
```

The `ca.crt` is used both for the nova-api connection and (by default) for
the Keycloak issuer's TLS — see `--oidc-ca-cert` in the manifests if your
Keycloak uses a different CA.

#### Rotation

Re-run `create-csi-client.sh` (it rotates the secret), update the Secret in
place with the new value, then:

```
kubectl -n nova-csi rollout restart deployment/nova-csi-controller daemonset/nova-csi-node
```

Because access tokens are refreshed every few minutes, **the access token
itself never needs to be rotated by hand** — only the client secret.

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

### CrashLoopBackOff with "no token provided" or "oidc setup failed"

Secret missing or wrong shape. Check
`kubectl -n nova-csi get secret nova-csi-auth -o yaml` and confirm the
`oidc-client-id`, `oidc-client-secret`, and `ca.crt` keys all exist and
are base64-encoded.

### "oidc: token endpoint returned 401: invalid_client"

The client secret in the Secret no longer matches Keycloak. Re-run
`deploy/keycloak/create-csi-client.sh` to mint a fresh secret and update
the Secret in place.

### "oidc: token endpoint returned 403: ... forbidden"

The `nova-csi` client's service account does not have the `nova-operator`
realm role. The setup script assigns it; if you bypassed the script,
assign it manually in the Keycloak admin UI under
*Clients → nova-csi → Service account roles*.

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
    sh -c 'curl -k --cacert /etc/nova-csi/ca.crt https://127.0.0.1:8444/healthz'
```

If this fails, the systemd nova-api isn't listening on 127.0.0.1:8444
or the pod isn't actually using `hostNetwork: true`.
