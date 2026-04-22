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
