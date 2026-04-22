# Security Policy

## Reporting a vulnerability

Please report suspected security issues privately to **security@novanas.io**
(placeholder address — TBD). Do **not** open a public GitHub issue or pull
request for anything you believe is a security vulnerability. We prefer
coordinated private disclosure and will acknowledge your report, agree on a
timeline, and credit you in the release notes unless you prefer otherwise.

## Supported versions

NovaNas follows the release model in
[`docs/13-build-and-release.md`](docs/13-build-and-release.md). Security fixes
are issued for:

- The **latest stable** release
- The **previous stable** release (overlap window)
- The **current LTS** release

Older releases receive fixes only on a best-effort basis. The `dev`, `edge`,
and `beta` channels are not covered by this policy; users on those channels
are expected to track tip.

## Scope

In scope:

- NovaNas appliance code in this repository (API, UI, operators, CLI,
  storage fork under `storage/`, installer, OS recipe, Helm charts, app
  catalog charts maintained here).

Out of scope:

- Third-party upstream components (Keycloak, OpenBao, Postgres, Redis, k3s,
  KubeVirt, Samba, Linux kernel, Debian packages, Grafana LGTM stack).
  Report those upstream; we will track their advisories and ship mitigations
  or version bumps as appropriate.
- Issues that require root on the appliance host or physical access beyond
  the documented threat model.

## Severity guidance

| Severity | Example |
|---|---|
| Critical | Remote unauthenticated code execution, data exfiltration across tenancy boundaries, bypass of encryption at rest |
| High | Authenticated privilege escalation, cross-user data access, persistent denial of service |
| Medium | Information disclosure with limited impact, authenticated denial of service, CSRF in privileged UI flows |
| Low | Security hygiene issues, non-exploitable defense-in-depth gaps, rate-limit weaknesses |

Classification is ultimately set by the NovaNas maintainers based on real-world
impact on an appliance in its supported configuration.

## Handling

Once a report is confirmed we will:

1. Acknowledge within 3 business days.
2. Develop and test a fix on a private branch.
3. Prepare a coordinated release across supported channels.
4. Publish an advisory after the fix is available, crediting the reporter.
