# ADR 0006: Monorepo with Nx orchestration

**Status:** Accepted
**Date:** 2026-04-22

## Context

NovaNas spans three language stacks (TypeScript, Go, Rust), a dozen
components (API, UI, CLI, operators, CSI, storage control plane, storage
dataplane, OS image, installer, Helm umbrella, app catalog), and a set of
shared contracts (Zod schemas, generated Go types, protobuf). Changes
routinely cross boundaries — a new CRD field touches schema, API, UI,
operator, and docs in one logical change.

Polyrepo layouts make atomic cross-layer changes hard: PRs have to land
in an ordered sequence across repos, generated types get out of sync,
and version negotiation between internal components becomes a job.
External dependencies (the Nova* stack, upstream Helm charts, Linux
packages) genuinely do belong in separate repos because they evolve on
their own cadence; internal code does not.

## Decision

All NovaNas-owned code lives in a **single monorepo** with **Nx**
orchestrating cross-language builds, caching, and affected-command
semantics. The stack-specific workspaces coexist:

- **pnpm workspaces** for TypeScript (`packages/*`)
- **Go workspaces** for Go (`packages/operators`, `packages/cli`,
  `storage/cmd/*`, etc.)
- **Cargo workspace** for Rust (`storage/dataplane` and any future
  Rust crates)

Nx sits above these and understands the cross-language dependency graph
(e.g., `api` depends on `schemas` whose build generates Go types
consumed by `operators`).

External Nova* projects (novanet, novaedge) stay in separate repos and
are consumed as OCI images and Helm subcharts.

## Alternatives considered

- **Polyrepo per component** — maximal isolation, maximal coordination
  cost, hostile to cross-layer changes.
- **Monorepo without Nx** — works for small projects; cross-language
  caching and affected-detection are the pain point as the tree grows.
- **Bazel** — strong hermeticity but a substantially larger tooling
  investment and a steep learning curve for contributors who do not
  already use it.

## Consequences

- Positive: atomic cross-layer PRs; generated types regenerated and
  committed in the same change that consumes them.
- Positive: shared tooling — one lint config per language, one CI
  pipeline, one set of hooks.
- Positive: Nx's affected-command semantics keeps CI fast as the repo
  grows.
- Negative: one big repo requires discipline around ownership and
  churn; we enforce this via workstream boundaries and
  [CONTRIBUTING.md](../../CONTRIBUTING.md).
- Negative: new contributors face three toolchains to install.
- Neutral: external Nova* remain independent, which is the right default
  for projects with their own users beyond NovaNas.

## References

- [docs/13-build-and-release.md](../13-build-and-release.md)
- [docs/14-decision-log.md](../14-decision-log.md) — R1, R2
- ADR 0007 — CalVer with SemVer API
