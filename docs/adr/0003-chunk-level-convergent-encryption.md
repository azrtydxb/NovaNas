# ADR 0003: Chunk-level convergent encryption

**Status:** Accepted
**Date:** 2026-04-22

## Context

Every NovaNas volume — block, file, or object — decomposes into 4 MB
immutable content-addressed chunks. Two user requirements are in tension:

- Dedup must work across volumes and tenants so storage efficiency is
  meaningful on a multi-tenant appliance.
- Data at rest must be encrypted with per-volume keys so that retiring a
  volume means making its data unreadable without touching the bytes on
  disk (crypto-erase), and so that key scope is bounded per volume.

Naive per-volume encryption kills dedup because identical plaintext
produces different ciphertext per volume. Global encryption preserves
dedup but gives every volume the same key, which loses crypto-erase and
scoping.

## Decision

NovaNas encrypts at the **chunk** layer using **AES-256-GCM in a
convergent scheme**. The encryption material for a chunk is derived
deterministically from the chunk plaintext plus a per-volume Data Key
(DK), so identical plaintext within a volume produces identical
ciphertext and therefore dedups naturally. A three-level key hierarchy
applies:

- **Master Key** — held in OpenBao Transit, never leaves OpenBao.
- **Data Key (DK)** — one per volume, wrapped by the Master Key.
- **Per-chunk key material** — derived from the DK and the chunk.

The Master Key is unsealed at boot via TPM, so the operator does not
type secrets on a cold start.

## Alternatives considered

- **Volume-level encryption with independent keys** — dedup impossible
  across volumes.
- **No encryption / application-layer only** — fails the appliance
  security posture and many compliance stories.
- **Single appliance-wide key** — dedup works, but crypto-erase and key
  scoping are lost; one compromise exposes everything.
- **Client-side encryption only** — incompatible with SMB/NFS and with
  server-side dedup.

## Consequences

- Positive: dedup preserved across volumes within the same DK scope.
- Positive: crypto-erase by destroying the DK; per-volume key scoping.
- Positive: Master Key never leaves OpenBao; TPM unseal keeps the boot
  UX appliance-like.
- Negative: convergent encryption is a confirmation-of-a-file style
  oracle if an attacker can guess plaintext; this is an accepted tradeoff
  given the single-appliance threat model.
- Negative: extra complexity in the chunk engine, in key rotation, and
  in backup/restore flows.
- Neutral: SSE-C (customer-supplied keys) objects are segregated into a
  non-dedup path — explicit user choice.

## References

- [docs/02-storage-architecture.md](../02-storage-architecture.md)
- [docs/14-decision-log.md](../14-decision-log.md) — S16, S17, S18
- ADR 0002 — Keycloak and OpenBao
