# 13 — Build & Release

## Repository structure

**Single NovaNas monorepo.** NovaStor source absorbed; no upstream dependency.

```
novanas/
├── api/                 # Fastify + TypeScript
├── ui/                  # React + Vite
├── cli/                 # novanasctl (Go, static binary)
├── operators/           # Go (controller-runtime) — NovaNas CRD operators
├── schemas/             # Shared Zod (TS) + generated Go types
├── storage/             # Forked from NovaStor
│   ├── cmd/             #   agent, meta, csi, scheduler, webhook, s3gw
│   ├── internal/        #   chunk, metadata, placement, operator, filer (to delete), s3
│   ├── dataplane/       #   Rust SPDK dataplane
│   └── api/v1alpha1/    #   CRD types → renamed to novanas.io group
├── proto/               # gRPC contracts (owned)
├── os/                  # RAUC bundle + Debian image build
├── installer/           # Text-mode curses installer (Rust or Go)
├── helm/                # NovaNas umbrella chart + sub-charts
├── apps/                # Official app catalog charts
├── docs/                # User + admin + developer docs
├── e2e/                 # End-to-end tests (Playwright + installer-in-VM)
└── .github/workflows/   # CI pipelines
```

## Workspaces and orchestration

**Monorepo + Nx** orchestrating multi-language workspaces:

- pnpm workspaces (TypeScript: api, ui, schemas, cli if TS tools)
- Go workspaces (operators, cli, csi, agent, meta, s3gw)
- Cargo workspaces (dataplane)
- Nx manages cross-language dependency graph, caching, affected-command semantics

## External dependencies

Pulled as-is, not vendored into monorepo:

| Project | Consumed as |
|---|---|
| novanet | OCI images + Helm subchart (separate repo) |
| novaedge | OCI images + Helm subchart (separate repo) |
| Keycloak, OpenBao, Postgres, Redis, k3s, KubeVirt, Prometheus/Loki/Tempo/Grafana | Upstream Helm charts (vendored as subcharts in our umbrella) |
| Debian, Linux, Samba | OS package / container base |

**Renovate bot** auto-PRs version bumps; CI validates; team reviews major-version jumps.

## Versioning

- **CalVer** for product release: `YY.MM.patch` (e.g., `26.07.3`) — appliance convention
- **SemVer** for API: `/api/v1`, `/api/v2` — maintained separately
- **CRD API versioning**: `v1alpha1` → `v1beta1` → `v1` as features stabilize; conversion webhooks for in-place upgrades

## Branching

- `main` — always green, continuously deployable
- `release/YY.MM` — cut at major/minor, backport-only
- Tags: `vYY.MM.patch` annotated and signed

## Release channels

| Channel | Cadence | Audience |
|---|---|---|
| `dev` / `edge` | Per-commit on main | Internal, early testers |
| `beta` | Monthly tag on release branch | Willing to test before stable |
| `stable` | Quarterly major, monthly patch | Default for production |
| `lts` | Long-term support for a release branch | Enterprise customers |

## CI platform

**GitHub Actions** with:
- Hosted runners for lint, unit, UI build
- **Self-hosted bare-metal runners** for E2E (hardware-shape bugs only surface there)

## CI pipeline per PR

```
├─ Lint & format (biome/TS, golangci-lint/Go, cargo fmt+clippy/Rust, helm lint, kubeconform/CRDs)
├─ Build (all targets, parallelized)
├─ Unit tests (vitest, go test, cargo test)
├─ Type check (tsc, proto lint, CRD schema validation)
├─ Container images built but not pushed (cached layers)
├─ Security scans (grype, gitleaks, semgrep, govulncheck)
├─ License scan (cyclonedx-cli, reject copyleft in runtime deps)
├─ UI: Playwright component tests + axe-core accessibility
├─ API: OpenAPI spec generated and diffed; breaking changes gated by `api-breaking` label
└─ Coverage report
```

## CI pipeline on main push (adds)

```
├─ Push container images → ghcr.io/azrtydxb/novanas/*:dev-{sha}
├─ Push Helm chart → ghcr.io/azrtydxb/novanas/charts/novanas:dev-{sha}
├─ Build OS RAUC bundle → dev channel
├─ Build ISO (x86_64) → uploaded
├─ Build VA images (OVA, qcow2) → uploaded
├─ E2E: boot ISO in QEMU, run installer, run smoke test suite
├─ S3-compat test suite (MinIO mint + Ceph s3-tests) against running image
├─ Performance regression benchmarks (fio, NFS/SMB throughput, S3 ops/s) vs previous dev baseline
└─ Auto-deploy to internal dev cluster of test appliances
```

## CI pipeline on release tag

```
├─ Freeze + full regression suite
├─ All artifacts built with release flag (opt-level, strip, embed SBOMs)
├─ cosign sign all container images, Helm charts, RAUC bundles, ISOs (keyless via GitHub OIDC + Fulcio/Rekor)
├─ RAUC bundles signed with offline NovaNas release key
├─ Generate + sign SBOMs (SPDX + CycloneDX)
├─ SLSA level 3 build provenance attestations
├─ Upload to beta channel initially; promote to stable after bake time
├─ Publish SDKs to npm, pypi, crates.io, proxy.golang.org
├─ Cut release notes from Conventional Commits log
└─ Publish docs site with version switcher
```

## Reproducible builds

- **Gated in CI**: CI fails if reproducibility drifts
- Go is natively reproducible with pinned toolchain
- Rust reproducible with pinned toolchain and build flags
- Node: lockfile discipline + esbuild bundle

## Container base

**Slim Debian** (not distroless) — has shell and tools for in-pod debugging while still small.

## Signing & supply chain

Non-negotiable for storage appliance:

- **cosign keyless** via GitHub OIDC + Fulcio/Rekor — every container, chart, RAUC bundle, ISO signed with transparency log
- **RAUC signing** uses an **offline NovaNas release key** (HSM or air-gapped, 2-of-3 custody). Different from cosign keys. CI builds unsigned; a human-gated release step performs signing on a trusted machine.
- **App catalog charts** signed with a separate key; unsigned charts must be in `community` or `custom` tier, not `official`
- **SBOMs** generated with syft, signed as cosign attestations
- **Kyverno** at pod admission time verifies signatures on the appliance (defense in depth)

## App catalog pipeline

Separate release cadence from the appliance:

```
apps/{app-name}/
├── chart/                 # Helm chart
├── metadata.yaml          # category, icon, description, schema
├── tests/                 # helm test + smoke scripts
└── versions/              # one subdir per chart version
```

CI on catalog changes:
1. Lint chart
2. Install to a test k3s cluster
3. Smoke-test (HTTP probe, basic functional test)
4. Sign with cosign
5. Publish → `oci://ghcr.io/azrtydxb/novanas-apps/{app-name}:{version}`
6. Update `AppCatalog` index

Appliances poll the catalog index per `AppCatalog.spec.refreshInterval`.

## Testing strategy

Layered pyramid:

1. **Unit** — per-package, every PR, seconds
2. **Integration** — kind cluster, real CRD reconciliation, mocked K8s where needed; minutes
3. **E2E** — QEMU/KVM: boot fresh ISO, run installer, pool/dataset/share/snapshot/replicate/restart cycle
4. **Compatibility** — AWS SDK smokes, MinIO mint, Ceph s3-tests, NFSv3/v4 compliance, SMB2/3 compliance (smbtorture), CSI conformance
5. **Upgrade** — install old release, RAUC-upgrade to current, verify state preserved
6. **Performance regression** — fio, NFS/SMB throughput, S3 ops/s; fails if regression > threshold

E2E runs on **self-hosted bare-metal runners** for hardware-shape realism.

## Artifacts produced

| Artifact | Format | Purpose |
|---|---|---|
| RAUC update bundles | `.raucb` signed | Online updates |
| ISO images | `.iso` hybrid, bootable | Bare-metal install |
| Virtual appliance images | `.ova`, `.qcow2`, `.vmdk`, raw | Virtualized deployments |
| Container images | OCI, amd64 | All NovaNas services |
| Helm charts | OCI | Umbrella + subcharts |
| App catalog charts | OCI, signed | Catalog entries |
| CLI binary | Static binary | `novanasctl` |
| SDKs | npm / pypi / go mod / rust crate | Generated from OpenAPI |
| SBOMs | SPDX + CycloneDX | Supply-chain transparency |

**v1 architecture scope: amd64 only.** arm64 plumbing kept in place but not shipped.

## Release flow

```
Day 0:  main @ {sha} → dev-{sha} → auto-E2E → internal dev appliances
Day 7:  cut release/26.07 branch, tag v26.07.0-beta.1 → beta channel
Day 14: v26.07.0-beta.2 (any fixes), longer bake on internal beta appliances
Day 21: v26.07.0 tagged → promoted to stable after sign-off
```

Patch releases (v26.07.1, .2, ...) on release branches for security/critical fixes, weekly cadence if needed.

## Key custody

- **RAUC signing key**: HSM or offline air-gapped, 2-of-3 team custody
- **cosign signing**: keyless via GitHub OIDC (no custody problem)
- **Root CA for internal PKI** (OpenBao): per-appliance, never leaves the box
- **App catalog signing key**: separate from RAUC, rotated independently

## Telemetry

Opt-in at first boot. When enabled, appliances send:

- Version information
- Hardware fingerprint (CPU class, RAM, disk count/types — no serials)
- Anonymized usage patterns (feature flags that triggered, error rates)
- Crash reports (redacted stack traces)

Backs two things:
- Which versions are in the field (drives backport decisions)
- Crash correlation (systemic bugs surfaced early)

## Dependency hygiene

- **Renovate bot** auto-PRs for deps (TS, Go, Rust, Helm, container bases)
- Major versions gated behind human review
- Security advisories trigger priority PRs
- Lockfiles checked in; pinned for reproducibility

## What CI does NOT do

- Deploy to customer production appliances (customers pull updates per their `UpdatePolicy`)
- Test on customer data
- Store long-term artifacts (GitHub release assets + ghcr.io only; old builds GC'd)
