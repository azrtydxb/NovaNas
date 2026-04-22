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

## Future enhancements

- Wire the NovaNas operators into a `kind` cluster alongside this stack so
  the API can reconcile real CRDs. Compose alone can't host the
  kube-apiserver the API talks to.
- Bring a mock storage backend. SPDK is not compose-friendly; a fake chunk
  agent binary would let us exercise the storage paths here.
