# ADR 0002: Keycloak for IAM and OpenBao for secrets

**Status:** Accepted
**Date:** 2026-04-22

## Context

A NAS appliance needs a full identity story — local users, groups, 2FA,
password policies, federation with Active Directory / LDAP / OIDC, and
SSO across the UI, CLI, API, and embedded Grafana. It also needs a
secrets management layer for service credentials, TLS material, an
internal PKI, and, critically, the master key that protects chunk-level
encryption (see ADR 0003).

Building either of these from scratch is a multi-year project in its own
right, and getting the details wrong has serious consequences. Both
concerns have mature open-source implementations with permissive licenses
that can be embedded inside the appliance.

## Decision

NovaNas uses **Keycloak** for all identity, authentication, and
federation, running a single realm (`novanas`) with groups. `User` and
`Group` CRDs are projections of Keycloak state, with Keycloak as the
source of truth. A custom NovaNas theme provides consistent branding on
the login page.

NovaNas uses **OpenBao** (Vault-compatible, OSS fork) for all secrets,
PKI, and the Transit engine. OpenBao is backed by the shared Postgres
instance so a single database backup covers it. Path-scoped ACLs
(portable to the OSS edition) are used rather than namespaces. The
chunk engine's master key lives in OpenBao Transit and is unsealed at
boot via TPM.

## Alternatives considered

- **Custom auth (e.g., better-auth, homegrown)** — was tried earlier and
  removed; federation, 2FA, and admin tooling are too much surface to
  own.
- **Dex + external IdP** — thinner, but leaves us without a local user
  database or admin UI.
- **HashiCorp Vault** — license concerns after the BSL shift; OpenBao is
  the community fork.
- **Separate secret stores per concern** — more moving parts, harder
  backup, duplicated operator effort.

## Consequences

- Positive: federation, 2FA, session management, password policy, admin
  tooling all come for free.
- Positive: one PKI, one secrets story, TPM auto-unseal gives the
  appliance UX ("no key on boot").
- Positive: `Certificate` CRD backed by OpenBao PKI plus ACME via
  novaedge covers every TLS need in one model.
- Negative: two heavyweight stateful services always running, both in
  the boot-order critical path.
- Negative: we must theme, configure, and upgrade Keycloak and OpenBao
  carefully across releases.
- Neutral: both are consumed as upstream Helm subcharts — we configure,
  we do not fork.

## References

- [docs/10-identity-and-secrets.md](../10-identity-and-secrets.md)
- [docs/14-decision-log.md](../14-decision-log.md) — I1 through I9
- ADR 0003 — Chunk-level convergent encryption
