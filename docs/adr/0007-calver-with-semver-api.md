# ADR 0007: CalVer for product, SemVer for API

**Status:** Accepted
**Date:** 2026-04-22

## Context

NovaNas ships as a single appliance: one image, one version number,
covering the OS, all services, Helm charts, UI, API, operators, and the
storage engine. The product's natural cadence is calendar-driven —
monthly patch releases, quarterly majors — and users think of it as
"the 26.07 appliance", not as a composition of component versions.

The REST API, on the other hand, is a **contract** with clients (UI,
CLI, SDKs, third-party scripts) that must be stable and evolvable
independently of the release calendar. Adding fields is non-breaking;
removing or renaming them is. Those semantics are what SemVer exists
to express.

Using one versioning scheme for both concerns would muddle them: SemVer
for the appliance overstates how much clients depend on internal
refactors, while CalVer for the API hides breaking changes behind a
date.

## Decision

- **Product version: CalVer** — `YY.MM.patch` (e.g., `26.07.3`). One
  version covers the entire appliance; users upgrade as a unit.
- **API version: SemVer** — conveyed in the URL path (`/api/v1`,
  `/api/v2`). Breaking changes require a new major path. The old major
  is supported for one release cycle after the next major ships.
- **CRD API version:** `v1alpha1` → `v1beta1` → `v1` as individual
  resources stabilize; conversion webhooks handle in-place upgrades.

Release branches are cut as `release/YY.MM`; tags are `vYY.MM.patch`
annotated and signed.

## Alternatives considered

- **SemVer for everything** — awkward for an appliance; users do not
  care whether a month's batch of fixes is "2.3.0" or "2.4.0".
- **CalVer for everything** — loses API stability signalling; clients
  cannot tell a breaking change from a monthly patch.
- **Rolling release without versions** — unacceptable for an appliance
  with support tiers (stable, LTS, backports).

## Consequences

- Positive: release notes and changelogs are generated cleanly from
  Conventional Commits per channel.
- Positive: API clients can pin to a major and upgrade deliberately.
- Positive: CRD evolution has a well-defined path with conversion
  webhooks.
- Negative: contributors must understand two schemes and apply each in
  the right place; CI gates breaking API changes behind an explicit
  label.
- Neutral: channel model (`dev` / `edge` / `beta` / `stable` / `lts`)
  layers on top of CalVer without modification.

## References

- [docs/13-build-and-release.md](../13-build-and-release.md)
- [docs/14-decision-log.md](../14-decision-log.md) — R3
- ADR 0006 — Monorepo with Nx
