# Workloads (Apps) — Operator Guide

NovaNAS ships a Helm-driven Apps subsystem so operators can install
curated applications (Plex, Jellyfin, Nextcloud, …) onto the embedded
k3s cluster directly from the NovaNAS HTTP API.

This document covers:

- the API surface (mirrored by the Web GUI Package Center)
- how to add custom charts to the index
- a worked example: install Plex, upgrade it, tail its logs, uninstall

## Architecture in 30 seconds

- Each app is a Helm release, deployed by `nova-api` against the
  embedded k3s cluster (kubeconfig at `/etc/rancher/k3s/k3s.yaml`).
- Each release lives in a dedicated namespace `nova-app-<release-name>`
  so removing the release also drops the namespace and all its
  resources.
- Helm release state is persisted in k3s itself via the secret-storage
  driver. NovaNAS does NOT ship its own helm storage backend — a
  freshly installed release is visible to `helm list -A` outside the
  API.
- A curated chart index ships at
  `deploy/workloads/index.json`. At install time `nova-api` reads
  `${WORKLOADS_INDEX_PATH:-/usr/share/nova-nas/workloads/index.json}`
  (set this env var on dev hosts to point at your repo checkout).

## Permissions

| Permission              | Granted to (default RoleMap) | Covers                                   |
|-------------------------|------------------------------|------------------------------------------|
| `nova:workloads:read`   | viewer, operator, admin      | catalog list/get, installed list/get/logs/events |
| `nova:workloads:write`  | operator, admin              | install, upgrade, uninstall, rollback, index reload |

## API surface

All paths are under `/api/v1`.

| Method | Path                                              | Permission              |
|--------|---------------------------------------------------|-------------------------|
| GET    | `/workloads/index`                                | `workloads:read`        |
| GET    | `/workloads/index/{name}`                         | `workloads:read`        |
| POST   | `/workloads/index/reload`                         | `workloads:write`       |
| GET    | `/workloads`                                      | `workloads:read`        |
| POST   | `/workloads`                                      | `workloads:write`       |
| GET    | `/workloads/{releaseName}`                        | `workloads:read`        |
| PATCH  | `/workloads/{releaseName}`                        | `workloads:write`       |
| DELETE | `/workloads/{releaseName}`                        | `workloads:write`       |
| POST   | `/workloads/{releaseName}/rollback`               | `workloads:write`       |
| GET    | `/workloads/{releaseName}/events`                 | `workloads:read`        |
| GET    | `/workloads/{releaseName}/logs`                   | `workloads:read`        |

## Worked example — installing Plex

The examples below assume a bearer token in `$TOKEN` and the API at
`https://nova.local`.

### 1. List the catalog

    curl -sH "Authorization: Bearer $TOKEN" \
      https://nova.local/api/v1/workloads/index | jq '.[].name'

You should see `plex`, `jellyfin`, `nextcloud`, `photoprism`,
`vaultwarden`, `syncthing`, `home-assistant`.

### 2. Install Plex with default values

    curl -sH "Authorization: Bearer $TOKEN" \
         -H "Content-Type: application/json" \
         -X POST https://nova.local/api/v1/workloads \
         -d '{"indexName":"plex","releaseName":"plex"}'

`releaseName` MUST match `[a-z0-9](-[a-z0-9])*` (DNS-1123 label rules,
plus a 53-char upper bound). The release lands in namespace
`nova-app-plex`.

### 3. List installed apps

    curl -sH "Authorization: Bearer $TOKEN" \
      https://nova.local/api/v1/workloads | jq

Each entry includes `name`, `namespace`, `chart`, `version`, `status`,
and `revision`.

### 4. See Plex's pods

    curl -sH "Authorization: Bearer $TOKEN" \
      https://nova.local/api/v1/workloads/plex | jq '.pods'

### 5. Tail Plex's logs

    curl -NsH "Authorization: Bearer $TOKEN" \
      "https://nova.local/api/v1/workloads/plex/logs?follow=true&tail=200"

The endpoint streams `text/plain` until you cancel.

### 6. Upgrade Plex (new values)

    curl -sH "Authorization: Bearer $TOKEN" \
         -H "Content-Type: application/json" \
         -X PATCH https://nova.local/api/v1/workloads/plex \
         -d '{"valuesYAML":"persistence:\n  config:\n    size: 10Gi\n"}'

You can also pass `version` to bump the chart version. Either field —
or both — must be set.

### 7. Roll back

    curl -sH "Authorization: Bearer $TOKEN" \
         -H "Content-Type: application/json" \
         -X POST https://nova.local/api/v1/workloads/plex/rollback \
         -d '{"revision":1}'

### 8. Uninstall

    curl -sH "Authorization: Bearer $TOKEN" \
         -X DELETE https://nova.local/api/v1/workloads/plex

This destroys the Helm release AND deletes the `nova-app-plex`
namespace, reclaiming any PersistentVolumes provisioned by the chart.

## Adding a custom chart

Edit `deploy/workloads/index.json` (or whatever
`WORKLOADS_INDEX_PATH` points at). Each entry needs:

```json
{
  "name": "myapp",
  "displayName": "My App",
  "category": "productivity",
  "chart": "myapp",
  "version": "1.2.3",
  "repoURL": "https://example.com/charts/",
  "defaultNamespace": "nova-app-myapp",
  "permissions": ["nova:workloads:write"],
  "defaultValues": {}
}
```

Required fields: `name`, `chart`, `version`, `repoURL`. Reload the
index either by restarting `nova-api` or by calling:

    curl -sH "Authorization: Bearer $TOKEN" \
         -X POST https://nova.local/api/v1/workloads/index/reload

The endpoint returns 200 on success or a 4xx with the parser error.

## Configuration knobs (env vars)

| Variable               | Default                              | Purpose                                                 |
|------------------------|--------------------------------------|---------------------------------------------------------|
| `WORKLOADS_KUBECONFIG` | `/etc/rancher/k3s/k3s.yaml`          | Where `nova-api` looks for the k3s kubeconfig.          |
| `WORKLOADS_INDEX_PATH` | `/usr/share/nova-nas/workloads/index.json` | Path to the curated chart index JSON file. |

## Troubleshooting

- **`/workloads/*` returns 503 with `no_cluster`** — `nova-api` could
  not reach the k3s API. On a fresh NAS, k3s may still be
  bootstrapping. Once `kubectl get nodes` works on the host,
  `nova-api` picks the cluster up automatically (no restart needed
  unless the kubeconfig path itself was wrong at startup).
- **`/workloads/*` returns 503 with `not_available`** — `nova-api`
  did not wire the workloads manager (e.g. the kubeconfig path was
  invalid AND we couldn't fall back to in-cluster config). Check
  the startup logs for `workloads:` lines.
- **Install returns 409 `already_exists`** — a release with the same
  name is present. Either pick a new `releaseName` or `DELETE` the
  existing one first.
- **Install hangs / pods stuck `Pending`** — the chart probably
  requested a StorageClass or LoadBalancer that k3s can't provision.
  `GET /workloads/{name}/events` surfaces the kubelet/scheduler
  reason directly.
- **Upgrade fails with `could not determine chart name`** — happens
  when an external operator installed the release outside NovaNAS so
  no NovaNAS-side metadata exists, and the in-cluster Helm storage
  has been pruned. Re-install via the API to restore metadata.

## Single-tenant for v1

The first release of this API is single-tenant. Multi-tenant install
(per Keycloak `nova-tenant` attribute, with per-tenant namespaces and
RBAC) is tracked separately as a follow-up.
