# Contributing to NovaNas

Thanks for your interest in contributing. NovaNas is a Kubernetes-native
single-node NAS appliance; its design is documented in [`docs/`](docs/).
This guide covers how to get set up, how we work, and what we expect from
a good contribution.

## Getting set up

Start with [`docs/developer/onboarding.md`](docs/developer/onboarding.md)
to go from `git clone` to a running dev environment. Prerequisites and the
bootstrap script are described there.

## Workstream ownership

NovaNas is built by a combination of humans and agent teams organized into
workstreams with strict file ownership. See
[`docs/15-implementation-plan.md`](docs/15-implementation-plan.md) for the
wave model and the ownership table.

Cross-boundary changes require an integration PR coordinated with the owning
workstream. Ad-hoc edits outside your workstream will be reverted.

## Branching

Branch from `main`. Use one of the following prefixes:

- `feat/<short-description>` — new feature
- `fix/<short-description>` — bug fix
- `chore/<short-description>` — tooling, deps, build, non-functional
- `docs/<short-description>` — documentation only
- `refactor/<short-description>` — internal restructure, no behavior change

Direct pushes to `main` are not allowed. All changes land via pull request.

## Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/). This is
not cosmetic — release notes and changelogs are generated from the commit
log per [`docs/13-build-and-release.md`](docs/13-build-and-release.md).

Format:

```
<type>(<optional scope>): <subject>

<optional body>

<optional footer(s)>
```

Common types: `feat`, `fix`, `chore`, `docs`, `refactor`, `test`, `build`,
`ci`, `perf`. Breaking changes append `!` after the type/scope or include a
`BREAKING CHANGE:` footer.

Examples:

```
feat(api): add volume snapshot endpoint
fix(ui): correct dataset quota rounding
docs: clarify RAUC bundle signing flow
refactor(storage)!: rename chunk ID field
```

## DCO sign-off

All commits must be signed off under the
[Developer Certificate of Origin](https://developercertificate.org/).
Append a `Signed-off-by:` line by committing with `-s`:

```sh
git commit -s -m "feat(api): add volume snapshot endpoint"
```

PRs with unsigned commits will fail CI.

## Pull request process

1. Open a PR against `main`. The PR template will prompt for a description,
   linked issues, and test notes.
2. CI must be green: lint, type-check, unit tests, security scans, license
   scan, and (where applicable) CRD schema validation.
3. Request review from the owning workstream. API-breaking changes also
   require the `api-breaking` label and API-workstream review.
4. Squash-merge once approved; preserve the Conventional Commit subject on
   the merge commit so the changelog picks it up.

## Coding standards

Tooling is enforced in CI. Local equivalents:

- TypeScript / JSON / Markdown formatting: [Biome](https://biomejs.dev/)
  via `biome.json`
- Go: `gofmt`, `golangci-lint`
- Rust: `rustfmt`, `clippy` (see `rust-toolchain.toml`)
- Helm: `helm lint`
- CRD schemas: `kubeconform`

Run formatters before pushing. CI failures on formatting are avoidable.

## Design changes

Non-trivial behavior or interface changes must update the relevant document
under [`docs/`](docs/). Significant decisions (architecture, cross-layer
contracts, supply chain, versioning) also warrant an ADR — see
[`docs/adr/README.md`](docs/adr/README.md).

The raw YAML escape hatch, the single-node assumption, and the chunk-level
encryption model are non-negotiable invariants documented in
[`docs/14-decision-log.md`](docs/14-decision-log.md). Changes that touch
them need an explicit team-lead sign-off before coding starts.

## Testing

See [`docs/developer/testing.md`](docs/developer/testing.md) for how to run
unit, integration, and E2E tests. In short:

- Unit: `pnpm -r test`, `go test ./...`, `cargo test`
- Integration: targets a `kind` cluster (wired up in Wave 3+)
- E2E: QEMU-based, runs against a RAUC-installed image (wired up in Wave 6)

Aim for ≥ 80% coverage on new code in your workstream.

## Licensing

NovaNas is licensed under Apache License 2.0 (see [LICENSE](LICENSE) and
[NOTICE](NOTICE)). By contributing, you agree your contributions are
licensed under the same terms, and you attest to the DCO via your
`Signed-off-by` line.

## Code of conduct

This project follows the [Contributor Covenant v2.1](CODE_OF_CONDUCT.md).

## Security issues

Do **not** report security issues in public issues or PRs. Follow
[SECURITY.md](SECURITY.md) for the private disclosure channel.
