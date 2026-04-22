# 15 — Implementation Plan & Team Composition

This document describes how the NovaNas design in docs 01–14 will be built. It defines:

- Workstreams (by domain, with strict file ownership)
- Waves (what runs in parallel vs sequentially)
- The agent team composition for each wave
- Integration points and merge protocols

## Reality check

NovaNas is a full appliance stack. Realistic scope estimates:

- Monorepo scaffolding + shared schemas: ~1–2 weeks
- API server + UI skeletons (routable, stubbed endpoints): ~2–4 weeks
- Storage fork + NovaNas-specific changes: ~4–8 weeks
- Operators for every CRD: ~4–8 weeks
- OS image + RAUC + installer: ~2–4 weeks
- Helm umbrella + subchart integration: ~1–2 weeks
- App catalog (30 apps): ~2–4 weeks
- E2E harness + CI hardening: ~2–3 weeks
- Polish, beta-bake, ship: ~4+ weeks

Total: ~6–12 months for a focused team. Agent teams accelerate the scaffolding and boilerplate-heavy work significantly but do not eliminate the design-review, integration-test, and bake cycles.

This plan covers the **first several waves of scaffolding**, yielding a repo that:

- Compiles / passes `pnpm build`, `go build ./...`, `cargo build`
- Has a working CI green on "hello world" state
- Has executable API server + UI responding to a basic auth+session flow
- Has scaffolded operators and CRDs
- Can be iterated on by ongoing agent teams or humans

## Repository structure (target)

```
novanas/
├── nx.json
├── pnpm-workspace.yaml
├── go.work
├── Cargo.toml                 # workspace
├── packages/
│   ├── schemas/               # @novanas/schemas (Zod + generated Go types)
│   ├── api/                   # @novanas/api (Fastify)
│   ├── ui/                    # @novanas/ui (React + Vite)
│   ├── cli/                   # novanasctl (Go)
│   ├── operators/             # Go controllers
│   ├── csi/                   # Go CSI driver
│   └── db/                    # @novanas/db (Drizzle schemas)
├── storage/                   # Forked from NovaStor
│   ├── cmd/ internal/ api/v1alpha1/
│   └── dataplane/             # Rust SPDK
├── proto/                     # gRPC protobuf contracts
├── os/                        # Debian image + RAUC build
├── installer/                 # Text installer
├── helm/                      # Umbrella chart
├── apps/                      # Catalog charts
├── e2e/                       # Playwright + QEMU
├── docs/                      # Already exists
├── .github/workflows/
├── .claude/                   # Agent-team state
└── hack/                      # Build scripts
```

## Workstreams (persistent ownership)

| Workstream | Owns | Depends on |
|---|---|---|
| **Platform** | `nx.json`, root configs, tooling, `.github/workflows/`, `hack/` | None |
| **Schemas** | `packages/schemas/` | Platform |
| **DB** | `packages/db/` | Schemas |
| **API** | `packages/api/` | Schemas, DB |
| **UI** | `packages/ui/` | Schemas |
| **CLI** | `packages/cli/` | Schemas (via generated Go types) |
| **Operators** | `packages/operators/` | Schemas (Go types) |
| **Storage Control Plane** | `storage/cmd/`, `storage/internal/`, `storage/api/v1alpha1/` | Schemas |
| **Storage Dataplane** | `storage/dataplane/` | proto |
| **Proto** | `proto/` | None (owned independently; consumers rebuild) |
| **OS** | `os/` | Storage, Operators, API, UI (container images consumed) |
| **Installer** | `installer/` | None |
| **Helm** | `helm/` | All component images |
| **Apps** | `apps/` | None (independent release cadence) |
| **E2E** | `e2e/` | Everything |
| **Docs** | `docs/` | None |

**Strict rule**: one workstream = one ownership boundary = one agent at a time. Cross-boundary changes go through integration PRs coordinated by the team lead.

## Implementation waves

### Wave 0 — Plan & seed (manual, done)

- Design docs 01–14 (✅)
- Design mockups (✅)

### Wave 1 — Monorepo foundation (parallelizable)

Agents spawned in parallel. Ownership non-overlapping.

| Agent | Owns | Deliverables |
|---|---|---|
| **A1-Platform** | Root, `nx.json`, `.github/`, `hack/`, top-level configs | pnpm/go/cargo workspaces, Nx config, shared tsconfig/biome/eslint/golangci/rustfmt, `.editorconfig`, `.gitignore`, CI skeleton (lint + unit), README with build instructions |
| **A1-Schemas** | `packages/schemas/` | `@novanas/schemas` package with Zod definitions for: Pool, Disk, BlockVolume, Dataset, Bucket, Share, Snapshot, User, Group, AppInstance, Vm, HostInterface, Certificate. Exported TS types + OpenAPI fragment generation setup. |
| **A1-Docs** | `docs/`, top-level `LICENSE`, `NOTICE` | Apache 2.0 license, NOTICE with NovaStor attribution, link `docs/README.md` from root `README.md` |

Exit criteria:
- `pnpm install && pnpm build` succeeds (schemas compiles)
- `go work init` succeeds (empty workspaces placeholder)
- `cargo check` on placeholder `storage/dataplane` succeeds
- CI green on all three

### Wave 2 — Component scaffolds (parallelizable, depends on Wave 1)

| Agent | Owns | Deliverables |
|---|---|---|
| **A2-DB** | `packages/db/` | Drizzle schema for: users (mirrored), sessions, audit_log, jobs, notifications, preferences, app_catalog_cache. Migration setup. |
| **A2-API** | `packages/api/` | Fastify app skeleton. Route tree for `/api/v1/*` (stubs). Zod-validated request/response. Keycloak OIDC client stub. Drizzle integration. Redis client. WebSocket gateway skeleton. Structured pino logging. `/api/version` endpoint works. |
| **A2-UI** | `packages/ui/` | Vite + React 19 + Shadcn/Tailwind + TanStack Router/Query setup. Route shells for all screens in docs. Auth flow wired to Keycloak OIDC. Login page. Dashboard skeleton. Design-files mockup styles adapted. |
| **A2-Operators** | `packages/operators/` | controller-runtime scaffolding. One controller placeholder per CRD kind. Main entrypoint. RBAC manifests. |
| **A2-CLI** | `packages/cli/` | Cobra skeleton for `novanasctl`. `login`, `version`, `whoami` commands that call the API. Static binary build. |
| **A2-Proto** | `proto/` | .proto files for chunk engine, metadata service, replication service. Buf lint + generate. |

Exit criteria:
- All packages build
- API server starts and responds to `/api/version` and `/api/health`
- UI dev server starts, login redirects to a mock OIDC endpoint
- Operators binary compiles
- CLI binary compiles and prints version

### Wave 3 — Storage fork (single agent, sequential)

This wave cannot be parallelized — importing and rewriting NovaStor code is one large coherent operation.

| Agent | Owns | Deliverables |
|---|---|---|
| **A3-Storage-Fork** | `storage/` | Copy NovaStor's entire tree into `storage/`. Rename Go module (`github.com/azrtydxb/novastor` → `github.com/azrtydxb/novanas/storage`). Rename API group `novastor.io` → `novanas.io`. Delete `internal/filer` (removed in our design). Adjust imports across the tree. Update Cargo crate names for dataplane. Apache 2.0 NOTICE preserved. |

Exit criteria:
- `go build ./storage/...` succeeds
- `cargo build` in `storage/dataplane/` succeeds
- All NovaStor unit tests that were passing still pass

### Wave 4 — Storage adaptations (parallel after Wave 3)

| Agent | Owns | Deliverables |
|---|---|---|
| **A4-Single-Node** | `storage/internal/metadata/`, `storage/internal/placement/` | Drop Raft on single-node (bootstrap=1 path or direct Badger). Flatten CRUSH to device-failure-domain. |
| **A4-Metadata-As-Chunks** | `storage/internal/metadata/`, `storage/dataplane/src/chunk/` | Implement "open chunk" state. Move Badger FSM storage to chunk engine. Superblock format. |
| **A4-Encryption** | `storage/dataplane/src/chunk/`, `storage/internal/agent/` | Convergent AES-256-GCM. Per-volume DK. OpenBao Transit integration. |

Coordination: these touch overlapping files. Sequence or have A4-Single-Node land first, then the others.

### Wave 5 — Helm + OS + Installer (parallel, depends on Wave 4 artifacts available as images)

| Agent | Owns | Deliverables |
|---|---|---|
| **A5-Helm** | `helm/` | Umbrella chart with subchart dependencies on storage, Keycloak, OpenBao, Postgres, Redis, Prometheus, Loki, Tempo, Grafana, novanet, novaedge, KubeVirt. Values templating. |
| **A5-OS** | `os/` | mmdebstrap recipe. RAUC manifest. First-boot scripts. systemd units. Signing workflow. |
| **A5-Installer** | `installer/` | Text-mode curses installer (Rust + ratatui). Partition, dd rootfs, write GRUB, post-install. |

### Wave 6 — Catalog + E2E (parallel)

| Agent | Owns | Deliverables |
|---|---|---|
| **A6-Apps** | `apps/` | Initial 30 catalog charts with metadata, icons, schemas. Signing pipeline. |
| **A6-E2E** | `e2e/` | Playwright harness. QEMU boot test. AWS SDK smokes. MinIO mint integration. |

### Wave 7 — Hardening (ongoing)

Parallel specialists added as needed: security audit, performance tuning, compat testing, docs authoring.

## Coordination protocol

**Team lead agent**: between waves, a coordinator agent audits deliverables, resolves cross-boundary questions, updates this plan, and signs off on wave completion.

**Integration PRs**: when a workstream needs a change outside its ownership, it opens a narrow integration PR, flags the owning workstream's agent for review, and merges only when green.

**File-level locks**: recorded in `.claude/team-state.md`; each agent writes its current files-in-flight before editing to avoid conflicts.

**Daily integration run**: CI runs full `pnpm build && go build ./... && cargo build` on main + every agent branch.

## Quality gates per wave

| Gate | Applied after |
|---|---|
| Build green on all three stacks | Every wave |
| Unit tests for new code ≥ 80% coverage | Wave 2+ |
| Lint clean (biome, golangci, clippy) | Every wave |
| Reproducible build check | Wave 3+ |
| Security scan clean (grype, gitleaks, semgrep, govulncheck) | Wave 2+ |
| OpenAPI spec drift check | Wave 2+ |
| CRD schema validation (kubeconform) | Wave 2+ |

## Out of scope for initial waves

- Multi-node clustering (design out of scope)
- arm64 builds (plumbing only)
- Visual design polish (separate design pass)
- Beta/stable release cadence (activated after Wave 7)
- Telemetry backend (design includes the client side, server side deferred)

## What each agent must read before starting

All agents must read:
- [docs/README.md](README.md)
- Their workstream's relevant design doc (01–14)
- [docs/14-decision-log.md](14-decision-log.md)
- This document

All agents must declare their file ownership before touching files. Deviations require team lead approval.
