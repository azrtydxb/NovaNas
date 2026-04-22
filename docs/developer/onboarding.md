# Onboarding

From `git clone` to a working dev environment.

## Prerequisites

| Tool | Version | Notes |
|---|---|---|
| Node.js | 22.x | Pinned via `.nvmrc`; use `nvm use` or Volta |
| pnpm | 9.x | `corepack enable` or install directly |
| Go | 1.23+ | For operators, CLI, CSI, storage control plane |
| Rust | stable | Toolchain pinned via `rust-toolchain.toml` |
| Docker | recent | For container builds and kind-based integration tests |
| QEMU/KVM | recent | Needed from Wave 6 onwards for OS/installer E2E |
| GPG | optional | If you want to sign commits in addition to DCO sign-off |

macOS and Linux are supported for development. Windows via WSL2 should work
but is not part of CI.

## Clone and bootstrap

```sh
git clone https://github.com/azrtydxb/novanas.git
cd novanas
./hack/bootstrap.sh   # checks toolchain versions, sets up hooks
pnpm install
```

`pnpm install` installs TypeScript workspace dependencies. Go and Rust
workspaces are built on demand by the relevant package commands.

## Monorepo layout

See [implementation plan](../15-implementation-plan.md#repository-structure-target)
for the canonical tree. Short version:

```
packages/    # TypeScript: schemas, api, ui, db, cli, operators, csi
storage/     # Forked NovaStor — Go control plane + Rust dataplane
proto/       # gRPC protobuf contracts
os/          # Debian image + RAUC bundle build
installer/   # Text-mode installer
helm/        # Umbrella chart + subcharts
apps/        # Catalog charts
e2e/         # Playwright + QEMU
docs/        # Design + user + developer docs
hack/        # Build/dev scripts
```

## Running individual packages

Nx orchestrates cross-language builds. Common commands:

```sh
pnpm --filter @novanas/schemas build
pnpm --filter @novanas/api dev
pnpm --filter @novanas/ui dev
go build ./packages/operators/...
cargo build -p novanas-dataplane
```

`pnpm -r test`, `go test ./...`, and `cargo test` run the unit suites for each
stack. See [testing.md](testing.md).

## Entry points by workstream interest

- **Storage engine** — `storage/dataplane/` (Rust), `storage/internal/chunk/`,
  design in [`docs/02-storage-architecture.md`](../02-storage-architecture.md)
- **Operators / CRDs** — `packages/operators/`, schemas in
  `packages/schemas/`, reference in
  [`docs/05-crd-reference.md`](../05-crd-reference.md)
- **API** — `packages/api/`, design in
  [`docs/09-ui-and-api.md`](../09-ui-and-api.md)
- **UI** — `packages/ui/`, same design doc
- **Identity / secrets** —
  [`docs/10-identity-and-secrets.md`](../10-identity-and-secrets.md)
- **OS / install / update** — `os/`, `installer/`, design in
  [`docs/06-boot-install-update.md`](../06-boot-install-update.md)

## Troubleshooting

- **pnpm version mismatch** — ensure `corepack enable` is active; pnpm is
  pinned in `package.json` via `packageManager`.
- **Node version wrong** — run `nvm use` (reads `.nvmrc`).
- **Empty Go / Cargo workspaces** — expected during Wave 1 scaffolding. The
  workspaces are declared but may contain no members until Wave 2–3.
- **`pnpm install` pulls mismatched versions** — delete `node_modules` and
  `pnpm-lock.yaml` only as a last resort; prefer
  `pnpm install --frozen-lockfile` to diagnose.
- **Missing generated code** — run `pnpm --filter @novanas/schemas build`
  first; several downstream packages depend on generated types.

## CLI shell completion

`novanasctl` and the `kubectl-novanas` plugin ship Cobra-generated
completion scripts for bash, zsh, fish, and PowerShell. See
[`packages/cli/README.md`](../../packages/cli/README.md#enabling-shell-completion)
for copy-paste install snippets.
