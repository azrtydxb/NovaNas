# 09 — UI & API

The single most visible part of the product. A NAS lives or dies on its UI.

**Important invariant**: The fact that NovaNas runs on Kubernetes is an implementation detail hidden from users. The UI never shows YAML, CRDs, or kube-api semantics. Users interact with a domain-shaped REST + WebSocket API served by the NovaNas API server.

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
| K8s client | `@kubernetes/client-node` |
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
    │ kube-apiserver      │   OpenBao   Prometheus
    │ (CRDs via SA token) │   Transit   query API
    └─────────────────────┘
```

## API server responsibilities

- **Domain shapes**: `/api/v1/datasets`, `/api/v1/shares`, `/api/v1/apps`, `/api/v1/pools`, `/api/v1/disks` — not CRD names
- **Composite operations**: single call creating dataset + share + snapshot schedule, transactional best-effort
- **Authoritative authorization**: resolves Keycloak token, checks user's scope, runs only what they're allowed to
- **Authoritative store** in Postgres: every business object (Datasets, Shares, Pools, Disks, Snapshots, Users, Groups, ApiTokens, Certificates, KmsKeys, network resources, …) plus sessions, audit log, jobs, preferences, app-catalog cache, metric rollups
- **Long-running op tracking**: jobs table; K8s events integrated; WebSocket subscription per job
- **Proxy endpoints**: SPICE over WS, log tailing, admin shell (via `kubectl exec` equivalent, audited)

## Where data lives

NovaNas's source-of-truth model: **Postgres is the source of truth for every business object**. `packages/api` is the only Postgres client. Kubernetes is the runtime that hosts our containers; it is *not* a database. CRDs that survive in the system are projection targets — workloads K8s itself runs (KubeVirt VMs, Helm-installed AppInstances, novanet/novaedge controllers) — not where business state lives.

| Data | Home | Durability |
|---|---|---|
| Business objects (Datasets, Shares, Pools, Disks, Snapshots, Users, Groups, ApiTokens, Certificates, KmsKeys, Bonds, Vlans, …) | Postgres `resources` polymorphic table — see `packages/db/src/schema/resources.ts` | Postgres dump |
| Identity glue | Keycloak (auth + group membership) → Postgres mirror | Postgres dump in config backup |
| Secrets, DKs, internal CA | OpenBao | OpenBao snapshot + TPM-/KMS-sealed unseal keys |
| Sessions, audit, jobs, prefs, app-catalog cache, metric rollups | Postgres (own schemas under `packages/db/src/schema/`) | Postgres dump |
| Session tokens, rate limits, WebSocket pub/sub | Redis | Ephemeral; rebuilds on restart |
| Workload-producing CRDs (KubeVirt VirtualMachine, AppInstance, NfsServer/SmbServer Pods, Ingress, FirewallRule, …) | etcd via kube-apiserver | k3s snapshots; same lifecycle as Pods |
| Chunk metadata (placement, volume maps) | BadgerDB on chunk engine (stored as chunks) | Storage engine |
| User data (volumes, buckets) | Chunk engine on data disks | Protection policy |
| Metrics, logs, traces | Prometheus / Loki / Tempo on NovaNas buckets | Self-hosted on chunk engine |

Postgres is on the **persistent partition of the OS disk** — survives chunk-engine issues; API stays up for diagnostics even during storage failures.

## Service-to-service auth (in-cluster)

Components that aren't a browser — disk-agent, storage-meta, storage-agent, samba/ganesha sidecar pollers, etc. — talk to the API the same way: HTTP request with their pod's projected ServiceAccount JWT in `Authorization: Bearer …`. The API forwards the token to the kube-apiserver's TokenReview endpoint, which verifies it and returns the SA identity. The mapping from `system:serviceaccount:<ns>:<sa>` to an internal NovaNas principal (with kind-targeted read/write scopes) lives in [`packages/api/src/auth/tokenreview.ts`](../packages/api/src/auth/tokenreview.ts). No new credentials are minted — we ride on the SA tokens kubelet already projects into every pod.

Why TokenReview rather than a separate token system: there's exactly one set of credentials per pod, with rotation handled by kubelet, and the kube-apiserver is already a trusted authority in this cluster. Adding a parallel token store buys nothing.

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
                 │ session   │ Bearer JWT   │ kubectl
                 │ cookie    │ (TokenReview)│ (admin only)
                 │           │              │
        ┌────────┴───┐  ┌────┴──────────┐  ┌┴──────────────┐
        │ Browser SPA│  │ disk-agent    │  │ K8s workloads │
        │ (TanStack) │  │ storage-*     │  │ that consume  │
        │            │  │ ganesha/samba │  │ surviving CRDs│
        │            │  │ network agents│  │ (KubeVirt,    │
        │            │  │ (poll API)    │  │  Helm, etc.)  │
        └────────────┘  └───────────────┘  └───────────────┘
```

Writes always start at the API. Postgres commits before any side effect runs. Side effects (Keycloak admin call, OpenBao Transit key creation, cert-manager Certificate submission, ConfigMap or sidecar-rendered config file) are best-effort: failures log but don't roll back the Postgres write. Eventual consistency on the projection side is reached either via the next API write touching that resource, or via an out-of-band reconciler.

## Auth flow

1. Browser hits any page → redirected to `/login` if no session cookie
2. `/login` initiates OIDC code+PKCE against Keycloak (branded as NovaNas login page via Keycloak theme)
3. Keycloak authenticates user (password, TOTP, WebAuthn, federated AD/LDAP/OIDC)
4. Callback to NovaNas API with authz code
5. API exchanges code for tokens; stores session in Redis; sets httpOnly secure SameSite=Strict cookie
6. SPA makes requests with cookie; API resolves session → user → permissions
7. Each request validated via Zod; authz checked against scopes; K8s ops performed with API's own SA
8. Every state change audited to Postgres and `AuditPolicy` sinks

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

Server owns K8s watches; fans out to WS clients via Redis pub/sub (survives API server replica scaling).

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
| CRD status changes | K8s watch → Redis pub/sub → WS → React Query invalidation |
| Performance metrics | API queries Prom / WS pushes current values |
| Log tailing | API tails container logs via `kubectl logs -f` equivalent → WS |
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
- Talks to the same NovaNas API (not kube-apiserver)
- Same auth: OIDC device code flow for interactive, API tokens for scripts
- Feature parity with the UI for common ops
- Exported commands for scripting backups, managing users, disk operations, etc.

## Packaging

- API server and UI shipped as a **container image bundle** in `novanas-system`
- `esbuild` bundle of the API into one JS file + distroless Node container
- UI static bundle served by novaedge directly (or baked into API pod — TBD)
- Helm-deployed, pinned to NovaNas release version

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
