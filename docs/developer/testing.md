# Testing

NovaNas uses a layered test pyramid, documented in full under
[`docs/13-build-and-release.md`](../13-build-and-release.md#testing-strategy).
This page is the short version for contributors.

## Layers

1. **Unit** — per-package, runs on every PR, completes in seconds.
2. **Integration** — real CRD reconciliation against a `kind` cluster;
   mocks used only for things that cannot run on kind. Minutes.
3. **E2E** — QEMU/KVM boot of a fresh ISO through installer and a smoke
   workflow (pool → dataset → share → snapshot → replicate → restart).
4. **Compatibility** — AWS SDK smokes, MinIO `mint`, Ceph `s3-tests`,
   NFSv3/v4, SMB2/3 (`smbtorture`), CSI conformance.
5. **Upgrade** — install previous release, RAUC-upgrade to current, verify
   state preserved.
6. **Performance regression** — fio, NFS/SMB throughput, S3 ops/s; fails
   above a configured drift threshold.

Layers 3–6 come online in Wave 6 onwards; layer 1 is expected in every PR
from the moment a package has executable code.

## Per-stack commands

| Stack | Unit tests |
|---|---|
| TypeScript | `pnpm -r test` (vitest) |
| Go | `go test ./...` |
| Rust | `cargo test --workspace` |

Filter examples:

```sh
pnpm --filter @novanas/api test
go test ./packages/operators/controllers/dataset/...
cargo test -p novanas-dataplane chunk::
```

## Integration tests

A `kind` harness is introduced in Wave 3. Once wired it will:

- Spin up a k3s-on-kind cluster
- Install the NovaNas CRDs
- Run the operator binary against the cluster
- Exercise reconciliation with table-driven test cases

Until then, controller-level tests use `controller-runtime`'s envtest.

## E2E tests

Playwright covers the UI; a QEMU-based harness boots the full ISO and runs
installer flows plus a smoke suite. Both live in `e2e/` and run on
self-hosted bare-metal runners in CI for hardware-shape realism.

## Conventions

- **TypeScript** — colocate tests as `*.test.ts` next to the source file.
  Use vitest; avoid jest-specific globals.
- **Go** — `*_test.go` in the same package. Prefer table-driven tests.
  Use `testify` only where it materially helps.
- **Rust** — `#[cfg(test)]` inline modules for unit tests; `tests/` for
  integration tests that exercise a crate's public API.

## Coverage

Target is **at least 80%** line coverage per workstream once the code
exists. CI produces a coverage report per PR; drops against the base branch
are flagged in the PR summary.

## Fixtures

- Deterministic seed data lives in each package under `testdata/` or
  `__fixtures__/`.
- Do not check in large binary fixtures; generate them at test time from
  small seeds where possible.
