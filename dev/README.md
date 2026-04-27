# NovaNas local dev stack

A `docker compose` setup that brings up NovaNas's external dependencies plus
the API and UI with hot-reload, so you can iterate against a fully browsable
instance without a Kubernetes cluster.

**Not for production.** See [`helm/`](../helm/) for real deployments.

## Prerequisites

- Docker Desktop (or any Docker Engine 25+ with Compose v2)
- Free ports: 3000, 5173, 5432, 6379, 8025, 8080, 8180, 8200, 9090

## Quick start

From the repo root:

```sh
make dev-up
```

Then open:

| Service         | URL                                                    | Creds                  |
| --------------- | ------------------------------------------------------ | ---------------------- |
| UI              | http://localhost:5173                                  | admin / admin-password |
| API             | http://localhost:8080 (health: `/health`, `/metrics`)  | Bearer from Keycloak   |
| Keycloak admin  | http://localhost:8180 (realm: `novanas`)               | admin / admin          |
| OpenBao         | http://localhost:8200 (root token `dev-token`)         |                        |
| Prometheus      | http://localhost:9090                                  |                        |
| Grafana         | http://localhost:3000 (profile `observability`)        | admin / admin          |
| Mailhog         | http://localhost:8025 (profile `mail`)                 |                        |

Seeded test users in the `novanas` realm:

| Username | Password          | Role   |
| -------- | ----------------- | ------ |
| admin    | admin-password    | admin  |
| user     | user-password     | user   |
| viewer   | viewer-password   | viewer |

OIDC clients:

- `novanas-ui` — public PKCE client, redirects `http://localhost:5173/*`
- `novanas-api` — confidential, client secret `dev-api-secret`
- `novanas-cli` — public, device-flow enabled

## Common commands

```sh
make dev-up        # start
make dev-down      # stop (keeps volumes)
make dev-logs      # tail logs
make dev-ps        # status
make dev-reset     # stop and wipe all volumes (fresh DB, fresh Keycloak)
```

Enable optional services via Compose profiles:

```sh
cd dev && docker compose --profile observability up -d   # + Grafana
cd dev && docker compose --profile mail up -d            # + Mailhog
cd dev && docker compose --profile full up -d            # everything
```

## Layout

```
dev/
├── docker-compose.yml        # all services
├── keycloak-realm.json       # auto-imported on Keycloak first boot
├── openbao-init.sh           # transit + KV v2 bootstrap
├── prometheus.yml            # scrape config
├── grafana-provisioning/     # datasources + dashboards
└── init-sql/                 # Postgres DB/user bootstrap
```

The `api` and `ui` services build from `packages/api/Dockerfile.dev` and
`packages/ui/Dockerfile.dev`. Source directories are bind-mounted so changes
on your host trigger `tsx watch` / Vite HMR.

## Troubleshooting

**Port conflict** — `docker compose ps` shows the offending service; either
stop the host process or edit the port mapping in `docker-compose.yml`.

**Keycloak won't import the realm** — the import runs only on a clean
database. Run `make dev-reset` to wipe volumes, then `make dev-up`.

**UI can't reach API** — confirm `api` is healthy: `docker compose logs api`.
The UI proxies `/api` and `/ws` to the api container (see
`packages/ui/vite.config.ts`).

**OpenBao init looped** — the init container exits once transit/KV are
configured. Re-run with `docker compose up -d openbao-init` if needed; the
script is idempotent.

## With Kubernetes (full stack)

For a full local-ish experience with a real kube-apiserver:

```sh
make dev-cluster-up
```

This creates a single-node [kind](https://kind.sigs.k8s.io/) cluster named
`novanas-dev` and brings up the compose stack wired to it. NovaNas
defines no CRDs (see [ADR 0005](../docs/adr/0005-hide-kubernetes-behind-api.md));
business state lives in the API server's Postgres, the cluster is
just a container runtime.

Extra prerequisites:

- [`kind`](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) v0.22+
- `kubectl` v1.29+

Browse:

- UI: http://localhost:5173
- API: http://localhost:8080
- Kubernetes API: `kubectl --kubeconfig=~/.kube/novanas-dev.kubeconfig.raw get all -A`
  (or just `kubectl --context kind-novanas-dev ...` — the context is
  merged into your default kubeconfig automatically)

Operators run in-cluster. The compose `api` service mounts
`~/.kube/novanas-dev.kubeconfig` at `/kube/kubeconfig` (server rewritten
to `host.docker.internal` so the container can reach the kind
apiserver), and proxies resource reads/writes to it.

Rebuild an image after source changes and reload it into the cluster:

```sh
make dev-load-image     # prompts for api / ui / operators
kubectl --context kind-novanas-dev -n novanas-system rollout restart deploy/novanas-operators
```

Tear it down with:

```sh
make dev-cluster-down   # stops compose + deletes the kind cluster
make dev-cluster-reset  # down + up again
```

### Kind layout

```
dev/kind/
├── kind-cluster.yaml   # 1-node cluster, ingress-ready, 8088/8443
├── create-cluster.sh   # kind create + kubeconfig rewrite
└── uninstall.sh        # kind delete + kubeconfig cleanup
```

### Networking gotcha

Inside the compose `api` container, `127.0.0.1` is the container itself,
not the host. `create-cluster.sh` therefore writes two kubeconfigs:

- `~/.kube/novanas-dev.kubeconfig.raw` — unmodified, use this on the host
- `~/.kube/novanas-dev.kubeconfig`     — `server:` rewritten to
  `host.docker.internal` with `insecure-skip-tls-verify: true` (the
  apiserver cert is issued for `127.0.0.1` only). This is the one
  mounted into the api container.

Dev-only. Never ship a kubeconfig with `insecure-skip-tls-verify: true`.

## Future enhancements

- Bring a mock storage backend. SPDK is not compose-friendly; a fake
  chunk agent binary would let us exercise the storage paths here.
- Ingress controller inside kind so UI/API can be exposed on
  `localhost:8088` the same way they will be in real clusters.
