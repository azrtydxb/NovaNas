# 14 — Decision Log

Consolidated list of every design decision locked in during the initial design review. Format: decision + short rationale.

## Foundation

| # | Decision | Rationale |
|---|----------|-----------|
| F1 | Default container runtime is k3s; runtime is swappable via a runtime adapter (Docker planned) | Avoid lock-in to a single runtime; appliance must run on whatever the deployment requires |
| F2 | The container runtime is an implementation detail, hidden from users; the API server is the sole source of truth and exposes zero CRDs | UX consistency; runtime portability; no second source of truth |
| F3 | Single-node only in v1; multi-box out of scope | Focused product; don't conflate clustering with appliance |
| F4 | Fork NovaStor, no ongoing upstream sync | NovaStor becomes independent; NovaNas owns its storage code |
| F5 | Immutable Debian OS with RAUC A/B partitions | Appliance model, atomic updates, rollback, familiar ecosystem |

## Storage architecture

| # | Decision | Rationale |
|---|----------|-----------|
| S1 | Everything is chunks (4 MB content-addressed, immutable) | Single engine backs block/file/object; dedup; proven model |
| S2 | Pool = bag of disks; no protection at pool level | Protection is per-volume |
| S3 | Protection per-volume, mutable, adaptive default | Flexibility matches user intent (e.g., photos EC, VM disks rep) |
| S4 | Disk roles: data \| spare only. No metadata/cache disk role. | Metadata is a chunk-volume; tiering is pool-to-pool datamover |
| S5 | Heterogeneous disk sizes within pool supported | CRUSH weights by capacity |
| S6 | Mixed disk classes warned, not blocked | Practical flexibility; UI surfaces tradeoff |
| S7 | Adaptive protection defaults at volume creation | Scales from 1 disk (rep×1) to 10+ (EC 8+2) |
| S8 | Strict write-quorum EC (no tail relaxation) | Durability > latency for v1 |
| S9 | Failure domain = device (single-node) | Obvious; CRUSH hierarchy flat by default |
| S10 | CRUSH hierarchy auto-detect: flat or enclosure-aware | Based on `/sys/class/enclosure` |
| S11 | Metadata stored as chunks on small-chunk pool | Everything-is-chunks invariant; no separate Raft StatefulSet |
| S12 | Drop Raft on single-node; BadgerDB FSM stored as chunks | Single-node doesn't need consensus |
| S13 | Open-chunk state (mutable, append-only, UUID-identified) for metadata WAL | Solves WAL-in-chunk-engine latency |
| S14 | Superblock per disk (~4 KB) as only non-chunk data | Bootstrap; CRUSH map location |
| S15 | Tiering v1: access-based promotion + age-based demotion | Real user value; chunk-engine already has datamover |
| S16 | Chunk-level encryption (AES-256-GCM, convergent) | Preserves dedup; crypto-erase; per-volume keys |
| S17 | Key hierarchy: Master Key → DK per volume → per-chunk key | Standard envelope model |
| S18 | TPM auto-unseal for master key (via OpenBao) | Appliance UX — no operator types keys on boot |
| S19 | OS installs on normal disks (not chunk engine) | Bootstrap separation; API stays up during storage issues |

## Access protocols

| # | Decision | Rationale |
|---|----------|-----------|
| A1 | Block device is the unit: iSCSI/NVMe-oF expose directly; NFS/SMB mount + export | Simple, reuses standards; drops NovaStor's custom filer |
| A2 | Drop NovaStor's `internal/filer` (custom NFS/VFS) | Use kernel NFS; no reason to maintain custom |
| A3 | NFS via host knfsd managed by a privileged operator pod | TrueNAS pattern; best performance on appliance |
| A4 | SMB via Samba in a userspace pod | Upstream Samba handles ACLs, oplocks, shadow-copies |
| A5 | Multi-protocol Share (one `share` API resource exports SMB + NFS together) | Matches typical NAS UX |
| A6 | xfs default, ext4 optional for formatted volumes | xfs better for large files (media, VMs) |
| A7 | S3 option A: native gateway, chunks direct | Preserves dedup; no MinIO license issues |
| A8 | Bucket = volume (peer of BlockVolume, Dataset) | Consistent mental model |
| A9 | Full AWS S3 API surface in v1 | Including Object Select, event notifications, Glacier semantics, S3 website, replication rules |
| A10 | Object Lock: enabled only at bucket creation (immutable after); no default mode; bypassGovernance as explicit user flag | AWS-compat; conscious choice |
| A11 | SSE translation: SSE-S3 → Bucket DK; SSE-KMS → `kmsKey` API resource; SSE-C → segregated, non-dedup | Fits convergent-encryption model |
| A12 | ACL model: per-Dataset `posix \| nfsv4`; UI auto-picks based on share type, Advanced exposes choice | Covers Linux-native and Windows-native needs |
| A13 | MinIO `mint` + Ceph `s3-tests` + AWS SDK smokes as CI quality gate | Objective compatibility bar |

## Tenancy & isolation

| # | Decision | Rationale |
|---|----------|-----------|
| T1 | Tenants: `novanas-system`, `novanas-users/*`, `novanas-vms`, `novanas-shared/*`, `novanas-apps-system` projected by the runtime adapter onto runtime-native scopes | System/user separation, runtime-agnostic |
| T2 | API-server admission, runtime-side privilege profile, novanet identity policy — all enforced | Defense in depth |
| T3 | User tenants default-deny via novanet | Zero-trust starting point |
| T4 | One UI, role-driven navigation | Simpler; authZ is the boundary, not the UI |
| T5 | NovaNas API server is the sole authZ boundary; runtime-native authorization is coarse and reserved for the API server / runtime adapter | Fine-grained auth in one place; runtime-agnostic |

## API model & runtime

| # | Decision | Rationale |
|---|----------|-----------|
| C1 | All resources are API-server-owned objects under `/api/v1alpha1/*`, backed by Postgres. No CRDs anywhere. | Single source of truth; runtime-pluggable; no spec/status drift |
| C2 | Resources are global by default for v1 (single-appliance) but carry a tenant field | Appliance simplicity, with a path to multi-tenant later |
| C3 | `dataset` is the unifying abstraction for file storage | Industry-standard mental model (ZFS, QNAP, Synology, NetApp) |
| C4 | Disk events: last-20 inline + Postgres event store for full history + `auditPolicy` long-term | Hybrid; UI-fast + audit-complete |
| C5 | `bucket` peers `dataset` + `blockVolume` | Consistent; object storage gets full protection/tiering/snapshots/replication |
| C6 | The runtime is reached via a single internal **runtime adapter** interface; k8s adapter today, docker adapter planned | Lets the rest of the system stay runtime-agnostic |

## Boot, install, update

| # | Decision | Rationale |
|---|----------|-----------|
| B1 | Immutable Debian + RAUC A/B + overlayfs for mutable paths | Debian-native immutable appliance pattern |
| B2 | Optional mdadm RAID-1 on boot disk | Admin choice |
| B3 | Text-mode curses installer | Broad HW compatibility |
| B4 | Ship ISO + OVA + qcow2 + raw + vagrant from day one | Dev/test use |
| B5 | Update channels: dev/edge/beta/stable/lts | Tiered release |
| B6 | Factory reset: soft / config / full (with secure erase on full) | Cover forgotten-password, reconfigure, retire scenarios |
| B7 | Config backup daily by default; destinations: dataset/cloud/email | Admin DR |
| B8 | Postgres on persistent partition (not chunk engine) | API server stays up during storage issues |
| B9 | Persistent partition ~80 GB (Postgres + OpenBao + logs + state) | Sized for Postgres growth |
| B10 | Bootstrap order: container runtime (k3s/docker) → Postgres → OpenBao (TPM unseal) → Keycloak → storage → novaedge/novanet → API → UI | Dependency-correct, runtime-agnostic |

## Disk lifecycle

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | 11-state machine, WWN/NAA identity | Survives slot moves |
| D2 | Admin-approved pool assignment; no auto | Multiple tiers; safety |
| D3 | Auto-promoted hot spares on failure | Reduce time-to-protection-restored |
| D4 | Dual-track failure recovery: immediate re-replication + standby for replacement | Protection restored without waiting for HW |
| D5 | Emergency protection downgrade allowed (audited) | Capacity-constrained rebuild escape |
| D6 | Rebuild rate limits: aggressive \| balanced \| gentle | Avoid ceph-style rebuild-tank-client-IO |
| D7 | Foreign imports: strict normal + explicit salvage mode | Safe default; real recovery path |
| D8 | SES/slot LEDs in v1 (enclosure view, click-to-blink) | Real-world disk-finding UX |
| D9 | Wipe priority: crypto-erase (if encrypted) > NVMe secure erase > SATA secure erase > multi-pass zero | Fast path when possible |

## Apps & VMs

| # | Decision | Rationale |
|---|----------|-----------|
| P1 | Three-tier catalog: official (signed) / community / custom | Governance + flexibility |
| P2 | Custom Helm charts allowed for advanced users | Power-user escape hatch |
| P3 | VM console: SPICE (via spice-html5) | Better perf + clipboard + sound |
| P4 | GPU passthrough in v1 (IOMMU + vfio-pci + GpuDevice CR) | Gaming/ML workloads |
| P5 | ~30 apps in initial official catalog | Starter set covers 80% of NAS use |
| P6 | App snapshot-before-update, 30-day retention | Safe updates |
| P7 | Apps/VMs as first-class sources for Snapshot, Replication, CloudBackup | Uniform data-protection |
| P8 | Network exposure: via novanet/novaedge; subdomain routing (`<app>.nas.local`) with wildcard TLS | Cleaner than path-prefix |

## UI & API

| # | Decision | Rationale |
|---|----------|-----------|
| U1 | React 19 + Vite + TanStack Router/Query/Table + Zustand | Mainstream; ecosystem depth |
| U2 | Shadcn/ui + Tailwind + Radix | Own the components; a11y built-in |
| U3 | Recharts (widgets) + ECharts (heavy detail) | Right tool per use case |
| U4 | Separate API server (Fastify) — K8s hidden behind domain-shaped REST+WS API | UX consistency; API owns composite ops |
| U5 | Drizzle ORM | SQL-first; lighter than Prisma |
| U6 | Postgres (not SQLite) + Redis where needed | Shared backend for Keycloak/OpenBao/API |
| U7 | No raw YAML editor in UI | K8s is hidden |
| U8 | Auth via better-auth removed → Keycloak OIDC | See identity topic |
| U9 | Single-binary (bundled) API server, containerized | Appliance simplicity |
| U10 | API versioning: `/api/v1/*` path; SemVer on API; old supported one cycle post-next | Stable contract |
| U11 | WebSocket everywhere (not SSE) | Bidirectional; multiplexed channels |
| U12 | Use as many public libs as needed; "code light as possible" | Minimize custom surface |
| U13 | Responsive-only for v1; no native mobile app | Push via ntfy/email suffices |
| U14 | Embedded Grafana for advanced monitoring; built-in graphs for primary dashboard | Best of both |
| U15 | CLI `novanasctl` talks to NovaNas API, not kube-apiserver | Same UX surface |

## Identity & secrets

| # | Decision | Rationale |
|---|----------|-----------|
| I1 | Keycloak for all IAM (users, groups, 2FA, OIDC, federation) | Industry-standard; federation for free |
| I2 | Single Keycloak realm (`novanas`) with groups | Simpler than per-tenant realms |
| I3 | Custom NovaNas theme on Keycloak login | Consistent branding |
| I4 | `user`/`group` API resources are projections of Keycloak (synced into Postgres) | Source of truth in Keycloak; stable IDs in API for audit/authZ |
| I5 | OpenBao for all secrets, PKI, Transit | Consolidates crypto/secrets concerns |
| I6 | OpenBao Postgres backend | Shared instance; one backup covers |
| I7 | OpenBao path-scoped ACLs (not namespaces) | Works in OSS; portable |
| I8 | Chunk engine master key in OpenBao Transit; chunks unwrap per-mount | Master key never leaves OpenBao |
| I9 | `certificate` API resource backed by OpenBao PKI / ACME via novaedge | One source for TLS material |

## Networking

| # | Decision | Rationale |
|---|----------|-----------|
| N1 | novanet as the workload-network layer; novaedge as LB/ingress/SD-WAN; both configured exclusively via the NovaNas API | In-house Nova stack; runtime-agnostic |
| N2 | nmstate for host network management, applied by a runtime-neutral host-agent | Declarative, runtime-portable |
| N3 | Discovery: mDNS + SSDP + WS-Discovery | Cover Mac, Linux, old and new Windows |
| N4 | Per-app DNS default on (`<app>.nas.local`) | Zero-config for users |
| N5 | novaedge handles ACME (Let's Encrypt); OpenBao stores key material | Preferred integration |
| N6 | IPv6 enabled by default when network provides it | Future-friendly |
| N7 | No default host firewall | Appliance default; admin opts into rules |
| N8 | `trafficPolicy` API resource unifies QoS (v1) | One model for interface/tenant/app/vm/job |

## Observability

| # | Decision | Rationale |
|---|----------|-----------|
| O1 | Grafana LGTM stack: Prometheus + Loki + Tempo + Grafana + Alloy | Coherent, battle-tested |
| O2 | Prometheus long retention (no Mimir) | Simpler at single-box scale |
| O3 | Tempo/tracing in v1 with 1% default sampling + debug-window toggle | Useful for cross-layer slow ops |
| O4 | `serviceLevelObjective` API resource in v1 | Admins who want them get them |
| O5 | Observability data self-hosted on NovaNas buckets (dog-food) | Exercise the product on the product |
| O6 | Curated ship-with alert ruleset enabled by default | Good UX out of the box |
| O7 | Per-app auto-scrape default on | Expected appliance behavior |
| O8 | Diagnostic support bundle encrypted against published support public key | Safe sharing |
| O9 | Health API hierarchical; drives UI top-bar badge | Users want green/yellow/red |
| O10 | Built-in charts via curated API query gateway; "Open in Grafana" for power users | Balances simple and advanced |
| O11 | Structured JSON logging with `component/request_id/user/resource_*/severity` fields | Searchable LogQL |

## Build & release

| # | Decision | Rationale |
|---|----------|-----------|
| R1 | Monorepo (NovaNas-owned code); external Nova* projects separate | Atomic cross-layer changes; Nova* evolve independently |
| R2 | Nx for multi-language workspace orchestration | TS + Go + Rust all in one repo |
| R3 | CalVer for product (`YY.MM.patch`), SemVer for API | Appliance convention + API stability |
| R4 | GitHub Actions, self-hosted bare-metal runners for E2E | Hardware-shape realism |
| R5 | Reproducible builds gated in CI | Supply chain guarantee |
| R6 | cosign keyless for containers/charts/ISOs; offline RAUC signing key (HSM, 2-of-3 custody) | Defense in depth |
| R7 | SBOMs + SLSA L3 provenance | Supply chain transparency |
| R8 | Slim Debian base (not distroless) | Shell/tools available for in-pod debug |
| R9 | amd64 only for v1; arm64 plumbing ready | Focus scope |
| R10 | App catalog signed separately from appliance; separate release cadence | Independent catalog evolution |
| R11 | Renovate for dep hygiene; human review for major bumps | Balance automation with care |
| R12 | Telemetry opt-in at first boot | Drives backport and crash-correlation decisions |

## Out of scope for v1

- Multi-box clustering
- arm64 shipping images (plumbing only)
- Web UI visual design (separate design pass after this documentation)
- Native mobile app
