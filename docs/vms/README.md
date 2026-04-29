# NovaNAS Virtual Machine Manager

This is the operator guide for the KubeVirt-backed VM management API
that powers the future "Virtual Machine Manager" pane in the NovaNAS
Web GUI (Synology VMM equivalent).

## Architecture

NovaNAS runs an embedded **k3s** cluster on every node. **KubeVirt**
provides the VM lifecycle (VirtualMachine + VirtualMachineInstance
CRDs) and **CDI** handles disk image imports (DataVolumes). Our
ZFS-backed CSI driver (`nova-csi`) supplies the underlying PVCs
through the `novanas-zvol` StorageClass.

The HTTP API (`nova-api`) wraps these CRDs behind a small DTO surface
under `/api/v1/vms*`, `/vm-templates`, `/vm-snapshots`, `/vm-restores`.
Operators never touch raw KubeVirt YAML.

### Per-VM namespace (`vm-<name>`)

Every VM lives in its own namespace, e.g. VM `dbserver` lives in
`vm-dbserver`. This:

- Lets RBAC scope cleanly (one Role per VM).
- Cascades a delete: removing the namespace removes the VirtualMachine,
  any VirtualMachineInstance, all PVCs, snapshots, etc.
- Avoids cross-VM blast-radius for a misconfigured CDI import.

The API enforces this — the `namespace` field on `POST /vms` defaults
to `vm-<name>` and the prefix is required when explicitly set.

### Console proxy

`GET /vms/{ns}/{name}/console` returns a **WebSocket URL** plus a
short-lived token. The browser opens the socket directly to virt-api;
nova-api does not proxy the stream. This keeps the API server cheap
and the latency low.

```json
{
  "wsUrl":     "wss://nas.example/k8s/apis/subresources.kubevirt.io/v1/namespaces/vm-x/virtualmachineinstances/x/vnc",
  "token":     "ey…",
  "expiresAt": "2026-04-29T12:34:56Z",
  "kind":      "vnc"
}
```

The token is minted with a default 5-minute TTL. The GUI is expected to
re-issue the call before expiry.

### Live migration

The `migrate` endpoint exists but returns **501 Not Implemented** on
single-node clusters (which is every NovaNAS install today). Once
multi-node clustering ships, the endpoint becomes a real call into
`virt-api`.

## RBAC

Two permissions are added in `internal/auth/rbac.go`:

| Permission     | Granted to                              |
|----------------|------------------------------------------|
| `nova:vm:read`  | viewer, operator, admin                  |
| `nova:vm:write` | operator, admin                          |

`vm:read` covers list, get, console-session minting (the WebSocket URL
is itself a credential — the UI needs it for read-only screen sharing).
`vm:write` covers CRUD, lifecycle (start/stop/restart/pause/migrate),
and snapshot/restore.

## Endpoints

| Method | Path                                          | Perm |
|--------|-----------------------------------------------|------|
| GET    | `/api/v1/vms`                                 | read  |
| POST   | `/api/v1/vms`                                 | write |
| GET    | `/api/v1/vms/{ns}/{name}`                     | read  |
| PATCH  | `/api/v1/vms/{ns}/{name}`                     | write |
| DELETE | `/api/v1/vms/{ns}/{name}`                     | write |
| POST   | `/api/v1/vms/{ns}/{name}/start`               | write |
| POST   | `/api/v1/vms/{ns}/{name}/stop`                | write |
| POST   | `/api/v1/vms/{ns}/{name}/restart`             | write |
| POST   | `/api/v1/vms/{ns}/{name}/pause`               | write |
| POST   | `/api/v1/vms/{ns}/{name}/unpause`             | write |
| POST   | `/api/v1/vms/{ns}/{name}/migrate`             | write |
| GET    | `/api/v1/vms/{ns}/{name}/console`             | read  |
| GET    | `/api/v1/vms/{ns}/{name}/serial`              | read  |
| GET    | `/api/v1/vm-templates`                        | read  |
| GET    | `/api/v1/vm-snapshots`                        | read  |
| POST   | `/api/v1/vm-snapshots`                        | write |
| DELETE | `/api/v1/vm-snapshots/{ns}/{name}`            | write |
| GET    | `/api/v1/vm-restores`                         | read  |
| POST   | `/api/v1/vm-restores`                         | write |
| DELETE | `/api/v1/vm-restores/{ns}/{name}`             | write |

The full schemas live in `api/openapi.yaml` (search for `VM*`).

## Curated templates

The catalog is a flat JSON file at `deploy/vms/templates.json`,
deployed to `/usr/share/nova-nas/vms/templates.json`. v1 ships:

- `debian-12-cloud`     — Debian 12 generic cloud
- `ubuntu-24.04-cloud`  — Ubuntu 24.04 LTS server cloud
- `fedora-40-cloud`     — Fedora 40 generic cloud
- `alma-9-cloud`        — AlmaLinux 9 generic cloud
- `windows-11`          — placeholder; operator must supply ISO + key

Override the path at startup with `VMS_TEMPLATES_PATH`.

### Windows 11

Microsoft does not redistribute Windows 11 cloud images. The template
entry exists so the GUI can present the option; the operator must:

1. Upload a Windows 11 ISO to a location nova-api can fetch over HTTP.
2. Provide a valid Windows 11 license key during provisioning.
3. Pass the ISO URL as a boot disk:

```json
{
  "name": "winbox",
  "templateID": "windows-11",
  "cpu": 4,
  "memoryMB": 8192,
  "disks": [
    { "name": "boot", "sizeGB": 64, "source": "url:https://internal/win11.iso", "boot": true }
  ]
}
```

## End-to-end walkthrough

The following uses `curl`; in practice the GUI does the same calls.

### 1. List the catalog

```sh
curl -sS -H "Authorization: Bearer $TOKEN" \
  https://nas.example/api/v1/vm-templates | jq
```

### 2. Create a VM from a template

```sh
curl -sS -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "name": "alpha",
        "templateID": "debian-12-cloud",
        "cpu": 2,
        "memoryMB": 2048,
        "cloudInit": {
          "user":     "operator",
          "sshKeys":  ["ssh-ed25519 AAAAC3…"],
          "hostname": "alpha"
        },
        "startOnCreate": true
      }' \
  https://nas.example/api/v1/vms
```

The API creates the namespace `vm-alpha`, builds a VirtualMachine with
a DataVolume sourced from the template, and (because `startOnCreate`
was true) also flips `running=true`.

### 3. Start an existing VM

```sh
curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  https://nas.example/api/v1/vms/vm-alpha/alpha/start
# 202 Accepted
```

### 4. Get a console URL

```sh
curl -sS -H "Authorization: Bearer $TOKEN" \
  "https://nas.example/api/v1/vms/vm-alpha/alpha/console?kind=vnc" | jq
```

The GUI opens `wsUrl` directly in a noVNC widget, sending `token` as
the bearer.

### 5. Snapshot it

```sh
curl -sS -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"namespace":"vm-alpha","name":"snap1","vmName":"alpha"}' \
  https://nas.example/api/v1/vm-snapshots
```

### 6. Restore from the snapshot

```sh
curl -sS -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "namespace":"vm-alpha",
        "name":"restore1",
        "vmName":"alpha",
        "snapshotName":"snap1"
      }' \
  https://nas.example/api/v1/vm-restores
```

### 7. Delete the VM

```sh
curl -sS -X DELETE -H "Authorization: Bearer $TOKEN" \
  https://nas.example/api/v1/vms/vm-alpha/alpha
# 204 No Content
```

The per-VM namespace `vm-alpha` is removed in the same call. PVCs,
snapshots, and any restore objects in the namespace are garbage-
collected by Kubernetes.

## Limits

- CPU: 1 — 64.
- Memory: 1 — 262144 MB (256 GB).
- Per-VM namespace name regex: `^[a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?$`.
- Console token TTL: default 5 minutes.

## Configuration knobs

| Environment variable    | Meaning                                                                |
|--------------------------|------------------------------------------------------------------------|
| `VMS_TEMPLATES_PATH`     | Path to the curated templates JSON. Default `/usr/share/nova-nas/vms/templates.json`. |
| `KUBEVIRT_KUBECONFIG`    | kubeconfig for talking to k3s/KubeVirt. Default `/etc/rancher/k3s/k3s.yaml`. |
| `KUBEVIRT_DISABLED`      | Set to `true` to disable the VM subsystem (handlers return 503).      |
| `VIRT_API_WS_BASE`       | Externally-reachable virt-api WebSocket base, e.g. `wss://nas.example/k8s`. |

## Troubleshooting

- **All `/vms*` endpoints return 503**. The KubeClient is not wired —
  either `KUBEVIRT_DISABLED=true`, or `KUBEVIRT_KUBECONFIG` does not
  resolve. Check `journalctl -u nova-api` for the warning at startup.
- **Console URL works but the browser can't connect**. Confirm
  `VIRT_API_WS_BASE` resolves from the operator's network. The token
  is bound to the VM and expires fast — re-fetch on every reconnect.
- **`POST /vm-snapshots` returns 404**. The VM does not exist or is in
  a different namespace. Snapshots are scoped to the VM's namespace
  (`vm-<vmName>` by convention).
- **Migrate returns 501**. Single-node cluster — multi-node migration
  is a follow-up.
