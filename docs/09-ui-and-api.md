# 09 — UI & API

The single most visible part of the product. A NAS lives or dies on its UI.

**Important invariant**: The container runtime (Kubernetes today, Docker tomorrow) is an implementation detail hidden from users. The UI never shows YAML, CRDs, or runtime-API semantics. Users interact with a domain-shaped REST + WebSocket API served by the NovaNas API server. The API server is the sole source of truth — there are no CRDs.

## Stack

### Frontend

| Concern | Choice |
|---|---|
| Build | Vite |
| Framework | React 19 |
| Router | TanStack Router |
| Data | TanStack Query + TanStack Table |
| State | Zustand (light) |
| UI | Shadcn/ui + Tailwind + Radix primitives |
| Forms | React Hook Form + Zod resolver (schemas shared with API) |
| Charts | Recharts (inline widgets), ECharts (heavy detail views) |
| Console | spice-html5, xterm.js |
| i18n | lingui |
| Icons | lucide-react |
| Auth (client) | `oidc-client-ts` against Keycloak |
| Testing | vitest + React Testing Library + Playwright E2E |

### Backend (API server)

| Concern | Choice |
|---|---|
| Language | TypeScript on Node.js 22+ |
| Framework | Fastify |
| ORM | Drizzle |
| DB | Postgres (shared with Keycloak and OpenBao) |
| Cache/sessions | Redis |
| Validation | Zod (via `@novanas/schemas`) |
| OpenAPI | `zod-to-openapi` → SDK generation |
| Auth | Keycloak OIDC (code + PKCE), session cookie, JWT claims → permissions |
| Runtime adapter (internal) | `@novanas/runtime` — k8s impl uses `@kubernetes/client-node`; docker impl uses `dockerode` |
| Metrics | `prom-client` (self) + `prometheus-query` (scrape Prom) |
| Logging | pino (structured JSON) |
| Testing | vitest + supertest + testcontainers |

## Architecture

```
┌──────────────────────────────────────────┐
│ Browser (React SPA)                      │
│   Shadcn/Tailwind, TanStack Query+Router │
│   Recharts + ECharts                     │
│   oidc-client-ts (Keycloak)              │
└──────────────┬───────────────────────────┘
               │ HTTPS + WebSocket (via novaedge)
┌──────────────▼───────────────────────────┐
│ novanas-api (Fastify + TypeScript)       │
│   - REST + WS endpoints                  │
│   - Keycloak OIDC                        │
│   - Drizzle → Postgres                   │
│   - Redis (sessions, pub/sub)            │
│   - Zod schema validation                │
│   - SPICE / log proxies                  │
│   - Audit writer                         │
└──────────────┬───────────────────────────┘
               │
    ┌──────────┴──────────┐
    │ Runtime Adapter     │   OpenBao   Prometheus
    │ (k8s impl: kube-    │   Transit   query API
    │  apiserver via SA;  │
    │  docker impl: dockerd │
    │  socket)            │
    └─────────────────────┘
```

## API server responsibilities

- **Domain shapes**: `/api/v1/datasets`, `/api/v1/shares`, `/api/v1/apps`, `/api/v1/pools`, `/api/v1/disks` — domain names, not runtime kinds
- **Composite operations**: single call creating dataset + share + snapshot schedule, transactional best-effort
- **Authoritative authorization**: resolves Keycloak token, checks user's scope, runs only what they're allowed to
- **Authoritative store** in Postgres: every business object (datasets, shares, pools, disks, snapshots, users, groups, apiTokens, certificates, kmsKeys, network resources, apps, VMs, …) plus sessions, audit log, jobs, preferences, app-catalog cache, metric rollups. There is no parallel state in the runtime — only ephemeral runtime objects (Pods, Services on K8s; containers on Docker) emitted by controllers.
- **Long-running op tracking**: jobs table in Postgres; controllers report progress via API writes; WebSocket subscription per job
- **Proxy endpoints**: SPICE over WS, log tailing, admin shell (via the runtime adapter — `kubectl exec` on K8s, `docker exec` on Docker; audited)

## Where data lives

NovaNas's source-of-truth model: **Postgres is the source of truth for every business object**. `packages/api` is the only Postgres client. The container runtime hosts our containers; it is *not* a database, *not* a state store, and *not* an extension API surface. NovaNas defines no CRDs. The runtime holds only ephemeral execution objects (Pods/Services on K8s, containers on Docker), all of which are recomputable from Postgres at any time.

| Data | Home | Durability |
|---|---|---|
| Business objects (datasets, shares, pools, disks, snapshots, users, groups, apiTokens, certificates, kmsKeys, bonds, vlans, apps, VMs, …) | Postgres `resources` polymorphic table — see `packages/db/src/schema/resources.ts` | Postgres dump |
| Identity glue | Keycloak (auth + group membership) → Postgres mirror | Postgres dump in config backup |
| Secrets, DKs, internal CA | OpenBao | OpenBao snapshot + TPM-/KMS-sealed unseal keys |
| Sessions, audit, jobs, prefs, app-catalog cache, metric rollups | Postgres (own schemas under `packages/db/src/schema/`) | Postgres dump |
| Session tokens, rate limits, WebSocket pub/sub | Redis | Ephemeral; rebuilds on restart |
| Runtime execution objects (Pods/Deployments/Services on K8s; containers/networks on Docker) | Runtime's own store (etcd on K8s, dockerd state on Docker) | Recomputable from Postgres by replaying the controllers |
| Chunk metadata (placement, volume maps) | BadgerDB on chunk engine (stored as chunks) | Storage engine |
| User data (volumes, buckets) | Chunk engine on data disks | Protection policy |
| Metrics, logs, traces | Prometheus / Loki / Tempo on NovaNas buckets | Self-hosted on chunk engine |

Postgres is on the **persistent partition of the OS disk** — survives chunk-engine issues; API stays up for diagnostics even during storage failures.

## Service-to-service auth (in-cluster)

Components that aren't a browser — disk-agent, storage-meta, storage-agent, samba/ganesha host-agents, controllers, etc. — talk to the API the same way: HTTP request with a workload identity token in `Authorization: Bearer …`. The runtime adapter mints these tokens: on Kubernetes via projected ServiceAccount JWTs verified through the kube-apiserver's TokenReview endpoint; on Docker via short-lived API-server-issued tokens injected as environment variables, verified locally against the API server's signing key. Either way, the principal-to-scope mapping is owned by [`packages/api/src/auth/`](../packages/api/src/auth/).

The principle: **workload identity is sourced from whatever the runtime adapter provides; the API server is the verifier and the scope authority**. Swapping the runtime swaps the verifier (TokenReview vs local JWT verify), not the policy model.

## Control flow

```
        ┌─────────────────────────────────────────────┐
        │  packages/api  (Fastify, source of truth)   │
        │                                             │
        │     ┌───────────┐      ┌───────────────┐    │
        │     │ PgResource│◄────►│   Postgres    │    │
        │     └─────▲─────┘      └───────────────┘    │
        │           │                                 │
        │     ┌─────┴─────┐                           │
        │     │ afterCreate│                          │
        │     │ /Patch hooks: synchronous projection  │
        │     │ to Keycloak / OpenBao / cert-manager  │
        │     └───────────┘                           │
        └────────▲───────────▲──────────────▲─────────┘
                 │ session   │ Bearer JWT   │ admin shell
                 │ cookie    │ (workload    │ (runtime-adapter
                 │           │  identity)   │  proxied; audited)
                 │           │              │
        ┌────────┴───┐  ┌────┴──────────┐  ┌┴──────────────┐
        │ Browser SPA│  │ disk-agent    │  │ Controllers   │
        │ (TanStack) │  │ storage-*     │  │ + runtime     │
        │            │  │ ganesha/samba │  │ adapter       │
        │            │  │ network agents│  │ (emit Pods or │
        │            │  │ controllers   │  │  containers)  │
        └────────────┘  └───────────────┘  └───────────────┘
```

Writes always start at the API. Postgres commits before any side effect runs. Side effects (Keycloak admin call, OpenBao Transit key creation, runtime-adapter calls to materialize containers/Pods, sidecar-rendered config file) are best-effort: failures log but don't roll back the Postgres write. Eventual consistency on the runtime side is reached by the controllers, which loop on API state and reconcile until the runtime matches.

## Auth flow

1. Browser hits any page → redirected to `/login` if no session cookie
2. `/login` initiates OIDC code+PKCE against Keycloak (branded as NovaNas login page via Keycloak theme)
3. Keycloak authenticates user (password, TOTP, WebAuthn, federated AD/LDAP/OIDC)
4. Callback to NovaNas API with authz code
5. API exchanges code for tokens; stores session in Redis; sets httpOnly secure SameSite=Strict cookie
6. SPA makes requests with cookie; API resolves session → user → permissions
7. Each request validated via Zod; authz checked against scopes; runtime ops performed by controllers via the runtime adapter using its own credentials
8. Every state change audited to Postgres and `auditPolicy` sinks

## API versioning

- Path-based: `/api/v1/*`
- New major version on NovaNas major release; old supported one cycle post-next
- Deprecation signaled in headers: `Deprecation: true`, `Sunset: <date>`
- `/api/version` endpoint publishes current + supported for SDK auto-negotiation

## WebSocket model

Single WS connection per browser tab. Multiplexed channels:

- `pool:{name}` — state, rebuild progress
- `disk:{wwn}` — SMART, state transitions
- `job:{id}` — long-running op progress
- `metrics:{scope}` — live throughput/IOPS streams
- `events` — audit / notification feed
- `app:{namespace}/{name}` — app state, update progress
- `vm:{namespace}/{name}` — VM state, console stream
- `console:{vm}` — SPICE binary channel

Server owns Postgres LISTEN/NOTIFY plus runtime-status feeds from the runtime adapter; fans out to WS clients via Redis pub/sub (survives API server replica scaling).

## Information architecture

Left-rail navigation:

```
Dashboard
Storage
  ├─ Pools
  ├─ Datasets
  ├─ Disks
  └─ Snapshots
Sharing
  ├─ Shares (SMB + NFS)
  ├─ iSCSI / NVMe-oF
  └─ S3 (ObjectStore, Buckets, BucketUsers)
Data Protection
  ├─ Snapshot schedules
  ├─ Replication
  └─ Cloud Backup
Apps
Virtual Machines
  ├─ VMs
  ├─ ISO library
  └─ GPU devices
Network
  ├─ Interfaces, VLANs, bonds
  ├─ DNS / mDNS
  └─ Firewall
Identity
  ├─ Users + Groups
  ├─ API tokens + SSH keys
  └─ Identity providers
System
  ├─ Settings
  ├─ Updates
  ├─ Certificates
  ├─ Alerts
  ├─ Audit log
  ├─ Support
  └─ Shutdown / reboot
```

For regular users (non-admin), the tree collapses automatically — based on RBAC — to:

```
My Dashboard / My Datasets / My Shares / My Snapshots / My Apps / My VMs
```

Same SPA, same components; visibility driven by API responses.

## Dashboard content

- Health banner (green/yellow/red)
- Per-pool capacity + sparklines
- Top active alerts
- Recent activity feed
- Performance sparklines (throughput, IOPS, network)
- App/VM running counts
- In-progress jobs with progress bars

## Charts strategy

- **Built-in graphs** — NovaNas API queries Prometheus, exposes typed `/metrics/...` endpoints. UI uses Recharts for dashboard widgets and ECharts for detail views with heavy time-series.
- **Advanced → Grafana** — embedded Grafana pre-provisioned with dashboards, accessible via "Open in Grafana" deep links. Full Grafana for power users who want custom queries and alerts.

## Real-time signals

| Signal | Path |
|---|---|
| Resource status changes | Postgres LISTEN/NOTIFY → Redis pub/sub → WS → React Query invalidation |
| Performance metrics | API queries Prom / WS pushes current values |
| Log tailing | API tails container logs via the runtime adapter (`kubectl logs -f` on K8s, `docker logs -f` on Docker) → WS |
| VM console | API proxies SPICE over WS → spice-html5 |
| Job progress | Jobs table polled + WS events on updates |

Consistent pattern: single WS connection, channel subscriptions, one "live store" React abstraction.

## Shared schemas

Single source of truth in `@novanas/schemas` workspace package:

```
@novanas/schemas    ← Zod definitions (shared)
@novanas/db         ← Drizzle tables + queries
@novanas/api        ← Fastify app (imports schemas)
@novanas/ui         ← React SPA (imports schemas)
@novanas/cli        ← novanasctl (imports schemas)
```

One schema = three consumers (API validation, UI forms, SDK types). OpenAPI auto-generated for docs and external SDKs.

## CLI — novanasctl

- Written in Go, static binary
- Talks to the same NovaNas API (never to the runtime — no kubectl, no docker CLI fallback)
- Same auth: OIDC device code flow for interactive, API tokens for scripts
- Feature parity with the UI for common ops
- Exported commands for scripting backups, managing users, disk operations, etc.

## Packaging

- API server and UI shipped as a **container image bundle** in the system tenant
- `esbuild` bundle of the API into one JS file + distroless Node container
- UI static bundle served by novaedge directly (or baked into the API container — TBD)
- Pinned to NovaNas release version; runtime-adapter installs them as native runtime objects (no Helm chart required at runtime)

## Accessibility and i18n

- WCAG 2.1 AA from day one, tested with axe-core in CI
- Keyboard-navigable everywhere; Radix primitives provide the foundation
- lingui for all user-visible strings; English at launch, plumbing ready for more

## What we don't build

- In-browser file manager — use `Filebrowser` as a catalog app
- In-app terminal as primary admin tool — offer a side panel only, not in main flow
- Grafana-lite dashboards — embed Grafana for deep monitoring
- Media player — use Plex/Jellyfin from the catalog

## Responsive / mobile

- Responsive breakpoints tested; dashboards and alerts usable on phones
- Push notifications via `AlertChannel` (ntfy, email, Pushover, browser push)
- Native mobile app is post-v1
