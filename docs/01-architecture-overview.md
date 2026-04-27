# 01 — Architecture Overview

> **⚠️ Storage-subsystem sections of this doc are out-of-date.**
> The Go agent / Go controller / multi-backend-type architecture described
> below has been replaced by an all-Rust three-daemon design
> (data / meta / frontend). For the current storage architecture see
> [`16-data-meta-frontend.md`](16-data-meta-frontend.md). The general
> principles, API server, UI, and non-storage components in this doc are
> still accurate.

## Guiding principles

1. **Everything is chunks.** Block volumes, filesystems, and objects all decompose into immutable 4 MB content-addressed chunks. One storage engine, three access surfaces.
2. **The API server is the single source of truth.** Postgres-backed, Fastify+Zod, every UI/CLI/SDK call goes through it. There is no second control plane.
3. **The container runtime is swappable.** Kubernetes is one implementation; a Docker (or other OCI runtime) backend must be reachable through the same controller surface. This means **no CRDs anywhere in NovaNas's own code** — neither user-facing nor "internal". Runtime-specific objects (Pods, Deployments, etc.) live behind a runtime adapter and are emitted *from* API state, never authored against.
4. **Strict layer separation.** Presentation → Chunk Engine → Backend. No layer may reach past the next; no policy engine runs in the data path.
5. **Single-node, not clustered.** v1 targets one physical appliance. Clustering is explicitly out of scope.
6. **Upstream-free.** NovaNas forks NovaStor once and owns its storage code from then on. No ongoing upstream sync.
7. **Appliance, not distro.** The hardware/OS/stack/UI are shipped together as one product with one version number.

## Layered architecture

```
┌──────────────────────────────────────────────────────────────────┐
│ Web UI (React SPA)                         CLI (novanasctl)      │
│   ↕ HTTPS + WebSocket                      ↕ HTTPS               │
├──────────────────────────────────────────────────────────────────┤
│ NovaNas API Server (Fastify + TypeScript)                        │
│   - Domain REST + WS, Zod-validated                              │
│   - Keycloak OIDC (auth), OpenBao (secrets)                      │
│   - Postgres (state), Redis (sessions, pub/sub)                  │
├──────────────────────────────────────────────────────────────────┤
│ NovaNas Controllers (Go) — runtime-neutral reconcilers            │
│   Read desired state from API server; converge runtime objects   │
│   for pools, volumes, shares, apps, VMs, networking, ...          │
│   ↕                                                              │
│ Runtime Adapter — Kubernetes (k3s) today / Docker / other OCI    │
│   Pods, Deployments, Services, ConfigMaps — emitted, not authored │
├──────────────────────────────────────────────────────────────────┤
│ Presentation Layer                                               │
│   iSCSI / NVMe-oF targets │ SMB (Samba pod) │ NFS (host knfsd)   │
│   S3 gateway (native chunks)                                     │
├──────────────────────────────────────────────────────────────────┤
│ Chunk Engine (Rust + SPDK)                                       │
│   - 4 MB content-addressed immutable chunks                      │
│   - Convergent AES-256-GCM encryption                            │
│   - CRUSH placement, replication / Reed-Solomon EC               │
│   - Owner fans out to replicas via gRPC                          │
├──────────────────────────────────────────────────────────────────┤
│ Backend Engine                                                   │
│   File (filesystem-backed) │ LVM │ Raw NVMe → SPDK bdevs        │
├──────────────────────────────────────────────────────────────────┤
│ Host OS: Immutable Debian, RAUC A/B, systemd, nmstate            │
└──────────────────────────────────────────────────────────────────┘
```

## Component inventory

### System services (runtime namespace / docker network: `novanas-system`)

| Component | Role | Binary / image |
|---|---|---|
| novanas-api | Domain API server (sole SoT) | Node/Fastify (TS) |
| novanas-ui | Static React SPA | Served by novaedge |
| novanas-controllers | Runtime-neutral reconcilers; read API state, emit runtime objects via the runtime adapter | Go |
| novanas-runtime-adapter | Pluggable backend: k8s (default) / docker | Go |
| novanas-csi | CSI driver | Go |
| novanas-scheduler | Data-locality scheduler | Go |
| novanas-webhook | Mutating admission webhook | Go |
| novanas-storage-meta | Metadata service (Badger FSM) | Go + gRPC |
| novanas-storage-agent | Per-node chunk agent | Go |
| novanas-storage-dataplane | SPDK chunk/replica engine | Rust |
| novanas-smb | Samba pod | Samba upstream |
| novanas-nfs-operator | Host knfsd manager | Go |
| novanas-s3gw | Native S3 gateway | Go |
| novanas-discovery | mDNS + SSDP + WS-Discovery | Avahi + custom |
| keycloak | IAM / IdP / OIDC | Upstream Keycloak |
| openbao | Secrets + PKI + Transit | Upstream OpenBao |
| postgres | API state + Keycloak + OpenBao backend | Upstream Postgres |
| redis | Sessions, pub/sub, cache | Upstream Redis |
| novanet | CNI (eBPF) | External Nova project |
| novaedge | LB, ingress, reverse proxy, SD-WAN | External Nova project |
| prometheus / loki / tempo / grafana / alloy | Observability | Upstream |

### User workloads (namespaces `novanas-users/*`, `novanas-vms`)

Installed apps (Helm-rendered) and KubeVirt VMs live here. Isolated from system namespaces via RBAC, Pod Security Admission, and novanet identity-based policy.

## Data flow

### Write path (block/file/object → storage)

1. Client request hits presentation layer (iSCSI target / Samba / knfsd / S3 gateway)
2. Presentation layer issues chunk read/write to chunk engine via gRPC
3. Chunk engine splits data into 4 MB plaintext chunks, hashes for chunk ID, convergent-encrypts with bucket/dataset DK
4. Placement engine (CRUSH) picks N devices per protection policy
5. Owner replica writes locally + fans out to peers via gRPC
6. Strict write-quorum ack returns up the stack; client sees success
7. Metadata service records the volume→chunk-list mapping in BadgerDB

### Read path (storage → block/file/object)

1. Client read reaches presentation layer
2. Presentation queries metadata for chunk list
3. Chunk engine reads chunks from any live replica, decrypts via DK, returns bytes
4. Presentation assembles and returns to client

### Control path (user action → system change)

1. User clicks in UI → React app calls API (REST or WS)
2. API validates via Zod, checks authZ via Keycloak claims
3. API persists desired state in Postgres (sole source of truth) and emits a change event on Redis pub/sub
4. NovaNas controllers observe the change, compute runtime intent, and call the runtime adapter to converge it; the adapter emits Pods/Deployments/Services on Kubernetes today, container/network primitives on Docker tomorrow
5. Controllers report observed status back into the API server (Postgres); the API streams it to all connected clients via WebSocket

No CRDs are involved at any step: every business object lives in Postgres, every runtime object is emitted from controller code, and the runtime is interchangeable.

## External dependencies

| Project | Consumption |
|---|---|
| novanet | OCI image + runtime manifests (external repo); configured via novanas-api, never authored as CRDs by users |
| novaedge | OCI image + runtime manifests (external repo); configured via novanas-api, never authored as CRDs by users |
| Keycloak | Upstream Helm subchart |
| OpenBao | Upstream Helm subchart |
| Postgres | Upstream Helm subchart |
| Redis | Upstream Helm subchart |
| Prometheus, Loki, Tempo, Grafana, Alloy | Upstream Helm subcharts |
| k3s | Upstream binary, installed as part of OS image |
| KubeVirt | Upstream Helm subchart |
| Samba | Upstream container image |
| Linux / Debian | Upstream base |

NovaStor source has been absorbed into the NovaNas monorepo under `storage/` and is no longer an external dependency.

## Single-version product

One version number (CalVer, e.g. `26.07.3`) covers the entire appliance: OS image + all components + Helm charts + UI + API + operators + storage engine. Users install or upgrade as a unit. No mix-and-match with component versions.
