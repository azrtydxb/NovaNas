# @novanas/api

The NovaNas domain API server — a Fastify application that exposes the
REST + WebSocket surface consumed by the NovaNas UI and CLI.

## Responsibilities

- REST API under `/api/v1/...` for pools, datasets, buckets, shares,
  disks, snapshots, apps, VMs, users, and system operations.
- WebSocket gateway at `/api/v1/ws` for live events (pool state, job
  progress, alerts, etc.).
- Keycloak OIDC authentication with Redis-backed sessions.
- Structured audit logging into Postgres via `@novanas/db`.
- Prometheus metrics (`/metrics`) and OpenTelemetry traces.

The API is the authZ boundary for everything the UI touches — see
`docs/04-tenancy-isolation.md` and `docs/09-ui-and-api.md`.

## Running locally

```bash
pnpm --filter @novanas/api dev
```

### Required environment variables

| Name                       | Default             | Notes                              |
| -------------------------- | ------------------- | ---------------------------------- |
| `NODE_ENV`                 | `development`       |                                    |
| `PORT`                     | `8080`              |                                    |
| `LOG_LEVEL`                | `info`              |                                    |
| `DATABASE_URL`             | —                   | Postgres URL                       |
| `REDIS_URL`                | —                   | Redis URL                          |
| `KEYCLOAK_ISSUER_URL`      | —                   | `https://.../realms/novanas`       |
| `KEYCLOAK_CLIENT_ID`       | —                   |                                    |
| `KEYCLOAK_CLIENT_SECRET`   | —                   |                                    |
| `SESSION_COOKIE_NAME`      | `novanas_session`   |                                    |
| `SESSION_SECRET`           | —                   | 16+ chars, used to sign cookies    |
| `API_PUBLIC_URL`           | `http://localhost:8080` | Used to build OIDC redirect URIs |
| `KUBECONFIG_PATH`          | —                   | Optional outside cluster           |
| `OPENBAO_ADDR`             | —                   | Optional scaffold-only             |
| `OPENBAO_TOKEN`            | —                   | Optional scaffold-only             |
| `PROMETHEUS_URL`           | —                   | Optional                           |

## Architecture

```
src/
  index.ts         bootstrap + graceful shutdown
  app.ts           Fastify app factory (testable)
  env.ts           Zod env parsing
  logger.ts        pino factory
  telemetry.ts     OpenTelemetry SDK
  plugins/         Fastify plugins (cors, helmet, cookie, auth, ...)
  routes/          Route modules (health, version, auth, pools, ...)
  services/        External clients (redis, db, kube, keycloak, prom)
  auth/            session store, RBAC, request decorators
  ws/              WS hub, pubsub bridge, channel registry
```

## Route layout

Implemented end-to-end:

- `GET  /health`, `/livez`, `/readyz`
- `GET  /metrics`
- `GET  /api/version`
- `POST /api/v1/auth/login`    — begin OIDC login
- `GET  /api/v1/auth/callback` — OIDC redirect_uri
- `POST /api/v1/auth/logout`
- `GET  /api/v1/me`
- `GET  /api/v1/ws`            — WebSocket gateway
- `GET  /docs`                  — Swagger UI

Scaffold (`501 Not Implemented` returning `{error: "not implemented", wave: 2}`):

- `/api/v1/pools/*`, `/api/v1/datasets/*`, `/api/v1/buckets/*`,
  `/api/v1/shares/*`, `/api/v1/disks/*`, `/api/v1/snapshots/*`,
  `/api/v1/apps/*`, `/api/v1/vms/*`, `/api/v1/users/*`,
  `/api/v1/system/*`.

Wave 3 fills these in against `@novanas/db`, `@novanas/operators`, and
the Kubernetes API.

## WebSocket channels

Channel patterns registered in `src/ws/channels.ts`:

| Pattern       | Description                             |
| ------------- | --------------------------------------- |
| `events`      | Kubernetes-event firehose               |
| `pool:*`      | Per-pool status, scrub progress         |
| `dataset:*`   | Dataset property changes                |
| `bucket:*`    | S3 bucket events                        |
| `share:*`     | Share config / client connection events |
| `disk:*`      | SMART + health updates                  |
| `snapshot:*`  | Per-dataset snapshot timeline           |
| `app:*`       | App lifecycle (install, update, etc.)   |
| `vm:*`        | VM lifecycle                            |
| `job:*`       | Long-running job progress               |
| `alert`       | System alert stream                     |
| `system`      | Global system status                    |

Client frame: `{op: 'subscribe'|'unsubscribe'|'ping', channel?: string}`.
Server frame: `{channel, event, payload}`.

## Building the container

```bash
docker build -t novanas/api:dev -f packages/api/Dockerfile .
```
