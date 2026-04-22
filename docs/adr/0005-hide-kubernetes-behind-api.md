# ADR 0005: Hide Kubernetes behind the NovaNas API

**Status:** Accepted
**Date:** 2026-04-22

## Context

NovaNas is sold and experienced as a NAS appliance, not as a Kubernetes
platform. Its users are admins and end users of storage services, not
cluster operators. Exposing `kubectl`, YAML, or CRD semantics directly in
the UI or CLI would:

- Bleed cluster-shaped concepts (pods, namespaces, StatefulSets) into a
  domain where users think in pools, datasets, shares, apps, and VMs.
- Force Kubernetes RBAC — which is coarse and cluster-scoped by default
  — to carry authorization for fine-grained, per-resource operations it
  was not designed for.
- Prevent us from shaping composite operations (e.g., "create a Dataset
  with a Share and a default Snapshot schedule") as single transactions
  with coherent validation and audit.
- Make future refactors difficult: anything users have scripted against
  the raw K8s API becomes a backwards-compatibility constraint.

## Decision

The **NovaNas API server is the sole user-facing control plane**. Every
admin UI action, CLI command, and SDK call goes through
`/api/v1/*` (Fastify, Zod-validated REST + WebSocket). The API server is
also the **authorization boundary**: it authenticates users via
Keycloak OIDC, applies fine-grained policy, writes CRDs through
kube-apiserver using its own service account, and pushes status back out
via WebSocket.

`novanasctl` talks to this API — not to kube-apiserver. The web UI has
no raw YAML editor. Kubernetes is an internal implementation detail.

## Alternatives considered

- **Thin UI wrapping kubectl/YAML** — fastest to build, worst UX; see
  Rancher.
- **Expose K8s API directly with RBAC** — pushes authorization into a
  model that does not express NovaNas's intent.
- **Two APIs (domain + raw YAML escape hatch)** — splits the contract,
  doubles the audit surface, encourages users to route around the
  domain API.

## Consequences

- Positive: consistent domain-shaped UX; users never see cluster
  concepts.
- Positive: API owns composite operations, validation, and audit.
- Positive: Kubernetes internals can be refactored without breaking a
  public contract.
- Negative: every feature must pass through the API — there is no
  "bypass for power users" in the normal flow.
- Negative: API server is on the critical path for every user operation
  and must be robust and fast.
- Neutral: a documented read-only `kubectl` escape hatch exists for deep
  debugging by engineers, but is not an official user interface.

## References

- [docs/09-ui-and-api.md](../09-ui-and-api.md)
- [docs/14-decision-log.md](../14-decision-log.md) — U4, U7, U15, T5
- ADR 0001 — Kubernetes base
