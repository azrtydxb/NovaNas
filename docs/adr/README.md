# Architecture Decision Records

NovaNas captures non-trivial design decisions as ADRs so that the *why*
survives the people who made the call. Rationale that is currently spread
across [`docs/14-decision-log.md`](../14-decision-log.md) and the numbered
design docs is distilled here, one decision per file.

## Style

We use a **MADR-lite** format: context, decision, alternatives considered,
consequences, references. Short is better; 200–500 words is plenty.

## Process

1. Pick the next unused number (zero-padded to four digits).
2. Create one file: `docs/adr/NNNN-short-kebab-title.md`.
3. Start with **Status: Proposed**. Open a PR.
4. On merge, either flip to **Accepted** (normal case) or leave as
   **Proposed** if further review is pending.
5. When a later ADR overrides an earlier one, mark the earlier as
   **Superseded by NNNN** and link both ways.
6. Add the new ADR to the index below.

ADRs are append-only history. Do not rewrite an accepted ADR to reflect a
later decision — write a new one that supersedes it.

## Template

```markdown
# ADR NNNN: Title

**Status:** Proposed
**Date:** YYYY-MM-DD

## Context
[1–2 paragraphs — what constraint / problem / tradeoff drove this]

## Decision
[1 paragraph — what was chosen]

## Alternatives considered
- Option A — one-line why not
- Option B — one-line why not

## Consequences
- Positive: ...
- Negative: ...
- Neutral: ...

## References
- Design doc links
- Related ADRs
```

## Index

| # | Title | Status |
|---|---|---|
| [0001](0001-kubernetes-base.md) | Kubernetes (k3s) as the base | Accepted |
| [0002](0002-keycloak-openbao.md) | Keycloak for IAM and OpenBao for secrets | Accepted |
| [0003](0003-chunk-level-convergent-encryption.md) | Chunk-level convergent encryption | Accepted |
| [0004](0004-fork-novastor-no-upstream-sync.md) | Fork NovaStor, no ongoing upstream sync | Accepted |
| [0005](0005-hide-kubernetes-behind-api.md) | Hide Kubernetes behind the NovaNas API | Accepted |
| [0006](0006-monorepo-nx.md) | Monorepo with Nx orchestration | Accepted |
| [0007](0007-calver-with-semver-api.md) | CalVer for product, SemVer for API | Accepted |
| [0008](0008-immutable-debian-rauc.md) | Immutable Debian with RAUC A/B updates | Accepted |
