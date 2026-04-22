# Workflow

The daily development loop and the rules around it.

## Daily loop

```
git pull --rebase           # stay current with main
git checkout -b feat/foo    # see CONTRIBUTING.md for branch prefixes
# edit within your workstream's owned files only
pnpm -r test                # or go test / cargo test as appropriate
git commit -s -m "feat(...): ..."   # DCO sign-off is required
git push -u origin HEAD
gh pr create
```

CI must be green before review. Squash-merge to `main`. Direct pushes to
`main` are blocked by branch protection.

## Wave model and file ownership

NovaNas is built in waves by a mix of humans and agent teams, each scoped to
a **workstream** with strict file ownership. Before editing a file outside
your workstream, open an integration PR and coordinate with the owning team.

See [implementation plan](../15-implementation-plan.md#workstreams-persistent-ownership)
for the full ownership table and the current wave.

From Wave 2 onwards, active agents record files-in-flight in
`.claude/team-state.md` to avoid merge conflicts. Humans working alongside
agents should check that file before starting edits in shared areas.

## Adding a new CRD

Every CRD touches several layers; skip a step and the UI or API will drift
from the operator. Checklist:

1. **Schema** — add the Zod definition in `packages/schemas/`.
2. **Go types** — regenerate with the schema build; do not hand-edit
   generated files.
3. **API route** — add request/response handlers in `packages/api/` under
   the appropriate `/api/v1/*` path; validate with the Zod schema.
4. **UI screen** — add the TanStack route + React components in
   `packages/ui/`; reuse shadcn primitives.
5. **Operator** — add or extend a controller under `packages/operators/`
   (or `storage/cmd/operator/` for storage-layer CRDs).
6. **Reference docs** — update
   [`docs/05-crd-reference.md`](../05-crd-reference.md) with the CRD shape
   and an example manifest.
7. **Tests** — unit tests per layer plus at least one integration test that
   exercises CRD create → reconcile → status.

CRD API versions follow `v1alpha1` → `v1beta1` → `v1` per
[`docs/13-build-and-release.md`](../13-build-and-release.md#versioning).
Breaking CRD changes require a conversion webhook.

## Dependency refreshes

- **Routine** — Renovate opens PRs for all stacks (npm, Go modules, Cargo,
  Helm chart versions, container bases). CI validates; a reviewer approves
  minor/patch bumps; major bumps go to the workstream owner.
- **Urgent** — for security advisories, maintainers can bypass Renovate and
  commit the bump directly with a `fix(deps):` commit. Follow
  [SECURITY.md](../../SECURITY.md) if the advisory is not yet public.

Lockfiles (`pnpm-lock.yaml`, `go.sum`, `Cargo.lock`) are always checked in.
