# NovaNas E2E test harness

This directory holds every test that requires a running NovaNas instance.
Unit and integration tests live in their respective packages; this is where
we go end-to-end.

The suites are layered per `docs/13-build-and-release.md`:

| Suite | Directory | What it does |
|---|---|---|
| UI / API E2E | `ui/` | Playwright drives a real browser against the UI and the API server. |
| API smoke | `api/` | Go HTTP probes across the 14 API route modules. |
| QEMU boot | `qemu/smoke-boot.sh` | Boots a built ISO, asserts installer + k3s + UI come up. |
| Upgrade | `qemu/upgrade-test.sh` | Installs a prior release, applies current RAUC bundle, verifies state. |
| S3 compat | `compat/s3/` | MinIO mint + Ceph s3-tests + AWS SDK (Python/JS/Go) smokes. |
| NFS compat | `compat/nfs/` | pynfs / cthon04 NFSv4 conformance. |
| SMB compat | `compat/smb/` | smbtorture SMB2/3 conformance. |
| Performance | `qemu/performance/` | fio, NFS, SMB throughput with regression gate. |

## Prerequisites

All suites
- **docker** (for MinIO mint, optionally for kind)
- **node 22** + **pnpm 9**
- **go 1.25+**

UI / API E2E
- `pnpm install` inside `e2e/`
- `pnpm exec playwright install --with-deps chromium`
- A reachable NovaNas at `$NOVANAS_BASE_URL` (default `https://localhost:8443`)

Cluster-driven runs
- **kind**, **kubectl**, **helm**
- Run `scripts/bootstrap-cluster.sh` to spin up a local NovaNas; tear down with `scripts/teardown.sh`.

QEMU boot / upgrade
- **qemu-system-x86_64**, **qemu-img**
- KVM acceleration preferred (`/dev/kvm`); falls back to TCG.
- A built ISO at `os/build/novanas.iso` (set `ISO=…` to override).

S3 compatibility
- MinIO mint: only needs Docker.
- Ceph s3-tests: Python 3 + build deps; the script creates its own venv.
- AWS SDKs: `pip install boto3`, `npm i @aws-sdk/client-s3 @aws-sdk/lib-storage`, `go mod` inside `compat/s3/aws-sdk/`.

NFS / SMB
- NFS: `nfs-common`, `python3` (for pynfs) or `make`+`gcc` (for cthon04).
- SMB: `samba-testsuite` providing `smbtorture`.

## Running locally

```bash
# 1) Spin up NovaNas in a kind cluster
bash scripts/bootstrap-cluster.sh

# 2) Install Playwright deps (first run only)
pnpm install
pnpm exec playwright install --with-deps chromium

# 3) Run the UI + API E2E
pnpm test                     # == playwright test
pnpm test --headed            # watch it run in a browser
pnpm test ui/tests/login      # filter by path

# 4) Run the Go API smokes
cd api && E2E_RUN=1 go test -v ./...

# 5) Run S3 compat against the local s3gw
make s3-mint s3-sdks

# 6) Tear down
bash scripts/teardown.sh
```

## Environment

| Var | Default | Notes |
|---|---|---|
| `NOVANAS_BASE_URL` | `https://localhost:8443` | UI+API target |
| `E2E_API_TOKEN` | unset | Bypasses OIDC in Playwright fixtures |
| `E2E_USERNAME` / `E2E_PASSWORD` | `e2e-admin` / `e2e-admin-password` | Used if no API token |
| `S3_ENDPOINT` | `https://localhost:9000` | Points at s3gw |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | `novanas` / `novanas-secret` | Seeded by `test-values.yaml` |
| `ISO` | `os/build/novanas.iso` | QEMU boot target |
| `NEW_BUNDLE` / `OLD_ISO` | — | Required for upgrade test |

## Interpreting failures

- **Playwright**: traces are kept on failure under `playwright-report/` and
  `test-results/`. Open the HTML report with `pnpm run report`.
- **QEMU**: serial console is streamed to `qemu/artifacts/serial.log`. The
  disk image `qemu/artifacts/vm-disk.qcow2` is left in place on failure —
  boot it manually to inspect state.
- **S3 mint**: per-suite logs under `artifacts/mint/`.
- **Ceph s3-tests**: pytest output plus a `.pytest_cache`; re-run one test
  with `pytest -k <name>` from the s3-tests checkout.
- **NFS / SMB**: per-tool logs under `artifacts/nfs/` and `artifacts/smb/`.
- **Performance**: CSVs under `artifacts/perf/`. The fio script commits its
  regression against `artifacts/perf/fio-baseline.csv`; refresh that file
  after intentional perf changes.

## Adding new scenarios

UI / API
1. Add a spec file under `ui/tests/`.
2. Import `test` and `expect` from `../fixtures/auth` (gives you an authed
   page and an `ApiClient`).
3. Use `../fixtures/seed` helpers rather than creating CRs inline — this
   keeps the fixtures idempotent and teardown-friendly.
4. Prefer `data-testid` over text selectors; extend `ui/lib/selectors.ts`
   rather than hard-coding.

Compatibility
- Drop a new script next to its peers; wire it into the `Makefile` and
  `.github/workflows/e2e.yml`. Write logs to `artifacts/<suite>/`.

## Upgrade-test fixtures

`qemu/upgrade-test.sh` installs a **prior release ISO** (passed as
`OLD_ISO=`) and applies a **current RAUC bundle** (passed as `NEW_BUNDLE=`).
In CI:

- `OLD_ISO` is pulled from the previous stable-channel release asset on
  `release-YY.MM` branches via the GitHub Releases API.
- `NEW_BUNDLE` is the artifact produced by the OS build job of the current
  workflow run.

Locally, you are expected to supply both. The test writes a marker file
under `/var/lib/novanas/e2e/marker` before upgrade and verifies it persists
across the RAUC install + reboot.

## Non-goals

- The harness does **not** build ISOs, RAUC bundles, or container images.
  Those come from the OS and platform pipelines.
- Playwright reports are **not** published externally. They live only as CI
  artifacts with a short retention window.
