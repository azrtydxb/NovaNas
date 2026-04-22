# ADR 0008: Immutable Debian with RAUC A/B updates

**Status:** Accepted
**Date:** 2026-04-22

## Context

A storage appliance must update safely or not at all. A partial upgrade
that leaves the system with mismatched kernel modules, a broken SPDK
build, or an incompatible CRD schema can lose user data — which is
unacceptable. Traditional package-based upgrades are non-atomic: any
failure mid-upgrade leaves the box in an undefined state that is hard to
recover from without physical access.

The proven pattern for appliances is an **immutable root with A/B
partitions**: updates are applied to the inactive slot, verified, then
booted atomically; a failed boot falls back to the previous slot.
ChromeOS, Android (treble), and several NAS/router products use
variations of this pattern.

NovaNas's existing dependency surface (Debian base containers, Samba,
knfsd, k3s, KubeVirt) and the skills of its target operators both point
to a Debian base rather than a custom distribution.

## Decision

NovaNas ships as an **immutable Debian** image built with `mmdebstrap`,
packaged as a **RAUC A/B** update bundle, with mutable paths (etc, var,
persistent state) on overlayfs over a dedicated persistent partition.
Boot uses GRUB with slot selection driven by RAUC; failed boots roll
back automatically.

Update bundles are signed with an **offline NovaNas release key** (HSM
or air-gapped, 2-of-3 custody). Container images, Helm charts, and ISOs
are separately signed keyless via cosign + Fulcio/Rekor. The two signing
chains exist because RAUC has a different threat model (offline bundle,
no internet assumption) than the OCI supply chain.

A persistent partition of roughly 80 GB holds Postgres, OpenBao, logs,
and API state — deliberately separated from the chunk engine so the API
and identity services remain reachable during storage issues.

## Alternatives considered

- **Package-based upgrades** — non-atomic, poor rollback, fragile on a
  storage appliance.
- **Custom/from-scratch image** — loses Debian ecosystem and hardware
  support.
- **Container-only OS (e.g., CoreOS/Flatcar)** — workable but a much
  narrower ecosystem for kernel modules and appliance-specific tooling.
- **ZFS boot environments** — ZFS is not in the NovaNas storage model
  and adds a kernel dependency we would not otherwise need.

## Consequences

- Positive: atomic updates with automatic rollback; recovery without a
  service call.
- Positive: clear boundary between immutable system and mutable state;
  factory reset is a well-defined operation.
- Positive: Debian package availability for drivers, firmware, and
  troubleshooting tools on a slim base image.
- Negative: two signing chains (RAUC offline + cosign keyless) increase
  supply-chain complexity; documented in
  [`docs/13-build-and-release.md`](../13-build-and-release.md).
- Negative: image build is heavier than a container build; reproducible
  builds must be gated in CI.
- Neutral: an optional mdadm RAID-1 on the boot disk is an admin
  choice, not a default.

## References

- [docs/06-boot-install-update.md](../06-boot-install-update.md)
- [docs/13-build-and-release.md](../13-build-and-release.md)
- [docs/14-decision-log.md](../14-decision-log.md) — F5, B1
