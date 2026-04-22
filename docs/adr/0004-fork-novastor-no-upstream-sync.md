# ADR 0004: Fork NovaStor, no ongoing upstream sync

**Status:** Accepted
**Date:** 2026-04-22

## Context

NovaStor is a general-purpose software-defined storage project: clustered,
protocol-rich, multi-node, with its own filer, its own Raft/consensus
layer for metadata, and a CRD group (`novastor.io`) scoped to SDS
deployments. NovaNas reuses a substantial portion of NovaStor's chunk
engine, metadata service, and dataplane, but diverges on fundamentals:

- Single-node only (no Raft needed).
- No custom filer — use kernel knfsd and upstream Samba.
- Chunk-level convergent encryption that NovaStor does not have.
- A different CRD group (`novanas.io`) aligned with the appliance product.
- An appliance release cadence (CalVer, monthly patches) that does not
  match NovaStor's.

Maintaining NovaNas as a living fork of NovaStor would require constant
upstream merges against a codebase that is being pulled in a different
direction. That cost buys us little, because the changes we want are
precisely the ones that do not belong upstream.

## Decision

NovaNas **forks NovaStor once** and treats the result as its own
codebase. The fork lives at `storage/` in the NovaNas monorepo. There
is no ongoing upstream sync, no cherry-pick routine, and no agreement to
accept upstream patches wholesale. NovaStor and NovaNas evolve
independently from the fork point.

Apache 2.0 attribution is preserved in NOTICE files; individual security
or correctness fixes may be cherry-picked ad hoc in either direction but
without any structural commitment.

## Alternatives considered

- **Stay upstream, contribute all NovaNas changes to NovaStor** — dilutes
  NovaStor's SDS focus; many NovaNas-specific choices (no filer,
  convergent encryption, single-node assumption) would be rejected.
- **Maintain a long-lived fork with routine upstream merges** — high
  ongoing cost, constant conflict resolution, little payoff given how
  much we diverge.
- **Reimplement from scratch** — throws away a working chunk engine and
  years of tested code.

## Consequences

- Positive: NovaNas can delete, rename, and restructure freely.
- Positive: single coherent monorepo with atomic cross-layer changes.
- Positive: CRD group rename to `novanas.io` removes namespace collision
  with NovaStor deployments.
- Negative: NovaStor bug fixes do not flow in automatically; we own
  everything under `storage/` from Day 1.
- Negative: attribution must be maintained carefully.
- Neutral: NovaStor is no longer a runtime dependency, only a historical
  ancestor.

## References

- [docs/01-architecture-overview.md](../01-architecture-overview.md)
- [docs/14-decision-log.md](../14-decision-log.md) — F4
- ADR 0001 — Kubernetes base
