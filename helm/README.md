# NovaNas Helm umbrella chart

This chart deploys the full NovaNas appliance on a k3s (or any Kubernetes
>=1.28) cluster. It assembles all NovaNas-owned workloads plus the
external dependencies listed in `Chart.yaml`.

## Contents

- `Chart.yaml` — subchart dependencies and umbrella metadata
- `values.yaml` — default values for every component
- `values.schema.json` — JSON Schema that `helm install` validates against
- `templates/` — NovaNas-owned resources (namespaces, deployments, RBAC,
  network policies, post-install jobs, alerts)
- `values/` — channel-specific override files (`dev`, `beta`, `production`)
- `tests/` — `helm test` pods

## Installation

```sh
# Fetch subcharts. Requires outbound access to:
#   - registry-1.docker.io (Bitnami)
#   - openbao.github.io
#   - prometheus-community.github.io
#   - grafana.github.io
#   - ghcr.io/azrtydxb (NovaNas-published charts: kubevirt, novanet, novaedge)
helm dependency update ./helm

# Lint + render
helm lint ./helm
helm template novanas ./helm > /tmp/rendered.yaml

# Install
helm install novanas ./helm \
  --namespace novanas-system --create-namespace \
  --values helm/values/values-production.yaml
```

## Values reference

See `values.yaml` — it is intentionally self-documenting. Each top-level
key maps to a component (`api`, `ui`, `operators`, `storage.*`, etc.) or
to a subchart (`keycloak`, `openbao`, `postgresql`, `observability`,
etc.). Subchart values are passed through verbatim; refer to the
upstream chart's README for the full schema.

### Channel overrides

| File                             | Purpose                           |
|----------------------------------|-----------------------------------|
| `values/values-dev.yaml`         | local dev — single replica, debug |
| `values/values-beta.yaml`        | beta channel — debug logs, 7d     |
| `values/values-production.yaml`  | production / stable               |

### Bootstrap secrets

`bootstrapSecret` in `values.yaml` seeds Postgres, Keycloak, and OpenBao
database passwords at first install. Leaving them empty causes the chart
to generate random values and keep them in a resource-policy=keep Secret.
After first boot the OpenBao init job migrates these into Transit-
wrapped entries; rotation is handled by the operators.

## Identity, secrets, observability content

The umbrella chart ships opinionated bootstrap content for the three
platform subsystems:

### Keycloak realm (`templates/keycloak-setup/`)
- `realm-configmap.yaml` — the `novanas` realm with four clients
  (`novanas-ui`, `novanas-api`, `novanas-cli`, `grafana`), four realm
  roles (`admin`, `user`, `viewer`, `share-only`), three groups, a
  strict password policy, brute-force protection, and a WebAuthn
  sub-flow required for admins.
- `theme-configmap.yaml` — the `novanas` login theme (parent: `keycloak`)
  with a dark palette CSS override and localized messages. Mounted into
  the Keycloak pod at `/opt/keycloak/themes/novanas/`.
- `post-install-job.yaml` — Helm post-install/upgrade hook. Uses
  `kcadm.sh` to import the realm and is idempotent.
- Toggle: `keycloak.realm.enabled` (default `true`).

### OpenBao policies & bootstrap (`templates/openbao-setup/`)
- `policies-configmap.yaml` — path-scoped ACL policies per
  `docs/10-identity-and-secrets.md`: `novanas-admin`, `novanas-api`,
  `novanas-storage-meta`, `novanas-storage-agent`, `novanas-s3gw`,
  `novanas-operators`, plus a reference template for per-user policies.
- `init-job.yaml` — enables Transit + PKI + kv-v2, configures the root
  CA, enables the Kubernetes auth method, binds every NovaNas
  ServiceAccount to its policy, and creates the chunk-engine master key
  (`transit/keys/novanas-chunk-master`). Idempotent.
- `k8s-auth-job.yaml` — placeholder (folded into `init-job.yaml`).
- Toggle: `openbao.policies.enabled` (default `true`).

### Grafana dashboards (`templates/grafana-setup/`)
- Ten dashboards provisioned via the sidecar (label
  `grafana_dashboard=1`): `system-overview`, `storage-pools`,
  `storage-disks`, `storage-datasets`, `protocols`, `apps`, `vms`,
  `network`, `cluster`, `security`.
- `datasources-configmap.yaml` — Prometheus (default), Loki, Tempo,
  Alertmanager, and a Postgres datasource for the audit log.
- Toggle: `grafana.dashboards.enabled` (default `true`). Subset via
  `grafana.dashboards.list`.

### Prometheus alerts (`templates/alerts/`)
- 28 rules across storage (disk/pool/scrub), data protection
  (snapshots/replication/cloud backup), node health, platform
  (OpenBao, Keycloak, certificates, API, operators, update), and
  security (failed logins, 2FA brute force, admin actions).
- Every rule is individually toggleable via `alerts.defaults.<ruleID>`.

## Development

```sh
# Iterate on a template without refetching subcharts:
helm template novanas ./helm --skip-tests | kubeconform -strict -

# Lint with values
helm lint ./helm --values helm/values/values-dev.yaml
```

## Subchart versions

Versions in `Chart.yaml` are the latest-known-good at time of writing
(April 2026). They are kept fresh by Renovate (`renovate.json`).
When bumping a major version:

1. Update `Chart.yaml`
2. `helm dependency update ./helm`
3. Diff the rendered output: `helm template ./helm > before.yaml` on old
   version, same on new, `diff`.
4. Run the smoke test.

## Troubleshooting

**`helm dep up` fails** — check registry access. On air-gapped installs
the subcharts should be vendored into `charts/` ahead of time.

**`helm lint` complains about missing subcharts** — run `helm dep up`.

**`helm template` errors about missing values** — ensure `values.yaml`
is present and has not been partially overridden; the schema requires
the top-level sections to be objects.

**Pods CrashLoopBackOff** — check the post-install Jobs first:

```sh
kubectl -n novanas-system get jobs
kubectl -n novanas-system logs job/novanas-openbao-init
kubectl -n novanas-system logs job/novanas-keycloak-setup
```

## Non-goals

- This chart does not ship Dockerfiles for NovaNas binaries — those live
  with their respective packages.
- It does not configure the host OS (disk partitioning, TPM, hugepages);
  that is the responsibility of the `os/` chart shipped separately.
- It does not run the first-boot installer; see `installer/`.
