# hack/

Developer and build scripts. Keep these small, POSIX-shell friendly, and
self-documenting.

| Script | Purpose |
|---|---|
| `bootstrap.sh` | Verify required toolchains (pnpm, go, cargo) are present and hydrate JS dependencies via `pnpm install`. |

Run from the repo root:

```sh
./hack/bootstrap.sh
```
