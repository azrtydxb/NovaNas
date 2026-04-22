# NovaNas CI Pipeline

This document describes the GitHub Actions pipeline that builds, tests,
signs, and releases NovaNas. Every workflow runs on GitHub-hosted
runners (`ubuntu-24.04`) — no self-hosted infrastructure required.

> **Status:** owner has paid GitHub Actions minutes. Larger runners
> (`ubuntu-24.04-4core`, etc.) are used where a workload benefits.

---

## At a glance

| Trigger          | Workflow(s)                                                               | Approx duration |
| ---------------- | ------------------------------------------------------------------------- | --------------- |
| PR (every push)  | `ci.yml`, `storage-spdk.yml` (build job), `os-build.yml`, `perf-gate.yml` | ~5-15 min       |
| Push to `main`   | all PR checks + `publish-charts.yml`, `os-build.yml` (artifacts retained) | ~30-45 min      |
| Nightly (cron)   | `s3-compat-nightly.yml`, `perf-nightly.yml`, `storage-spdk.yml` (integration) | 1-4 hours   |
| Tag push `v*`    | `release.yml` (full signed release)                                       | 2-6 hours       |

---

## Per-PR (fast feedback, < 15 min)

- **`ci.yml`** — Biome lint, golangci-lint, `cargo clippy`, typecheck, build, test. pnpm / Go / Cargo caches.
- **`storage-spdk.yml`** (`dataplane-build` job) — `cargo fmt --check`, `cargo clippy --no-default-features`, `cargo build --no-default-features`, `cargo test --no-default-features`. No SPDK linking on PR — that keeps PR times sane and avoids the prior SPDK-linking flakiness. Covers the Malloc bdev.
- **`os-build.yml`** — on PRs that touch `os/**` or `installer/**`, validates the full chain (base rootfs via mmdebstrap -> layered rootfs -> dev-signed RAUC bundle -> ISO -> VA images). Artifacts retained 14 days.
- **`perf-gate.yml`** — any PR touching `storage/`, `packages/dataplane/`, `e2e/qemu/performance/` or `perf/` must carry the `perf-regression-ok` label. The label means the author ran the perf suite (or rerun the nightly on their branch) and results are within baseline tolerance.

## Push to `main`

All PR checks plus:

- **`publish-charts.yml`** — every app chart (`apps/*/chart/**`) and the umbrella `helm/` chart are packaged, pushed to `ghcr.io/azrtydxb/novanas-apps` (umbrella -> `ghcr.io/azrtydxb/charts`) and cosign keyless-signed.
- **`os-build.yml`** — artifacts retained for dev consumption.

## Nightly

- **`s3-compat-nightly.yml`** (02:00 UTC) — spins up a `kind` cluster, `helm install novanas` with `global.image.tag=dev`, port-forwards the S3 endpoint and runs: MinIO mint, Ceph s3-tests, and the AWS SDK smoke scripts (`e2e/compat/s3/**`). On failure, auto-files an issue tagged `ci-failure,s3-compat,nightly`.
- **`perf-nightly.yml`** (04:00 UTC Sundays) — boots the QEMU fixture, runs `fio-baseline.sh`, `nfs-throughput.sh`, `smb-throughput.sh`, then `perf/compare.py --baseline perf/baseline.csv --results perf/results/ --threshold-pct 10`. Regressions > 10% fail the job and auto-file an issue.
- **`storage-spdk.yml`** (`dataplane-integration` job, 03:00 UTC) — full SPDK-linked build via `make -C storage/dataplane ci-integration` and integration tests via `make test-integration`. Uses KVM, 2048 hugepages.

## Tag push (release)

**`release.yml`** runs on `v*` tag push:

1. Build & push container images (`hack/ci/build-images.sh`): `api`, `ui`, `operators`, `storage-meta`, `storage-agent`.
2. **cosign sign containers** — keyless (Sigstore OIDC via GitHub-hosted runner; no long-lived keys).
3. Package and push every app chart + umbrella; cosign keyless sign each.
4. Build RAUC bundle + installer ISO + VA images (qcow2/vmdk/ova/raw) via `make -C os all`.
5. **RAUC sign bundle** (`hack/ci/rauc-sign-release.sh`) — beta uses `RAUC_SIGNING_KEY` / `RAUC_SIGNING_CERT` repo secrets. **GA must migrate to Cloud KMS** (see below).
6. Generate SBOM (`anchore/sbom-action`, SPDX JSON).
7. Create GitHub Release with `.raucb`, `.iso`, `.qcow2`, `.vmdk`, `.ova`, `sbom.spdx.json`.

---

## Runner sizes

All workflows target `ubuntu-24.04` — it provides KVM, root (via `sudo`), hugepages, modern Debian tooling (mmdebstrap, rauc, xorriso), Docker, and the standard Go/Rust/Node toolchains.

Upgrade to `ubuntu-24.04-4core` (or a larger paid runner) if any of these become too slow:

- `os-build.yml` `va-images` job — Packer + QEMU is CPU-bound.
- `storage-spdk.yml` `dataplane-integration` job — full SPDK build.
- `release.yml` — the monolithic pipeline benefits from 4+ cores.

---

## Required secrets

| Secret                   | Scope         | Purpose                                                    |
| ------------------------ | ------------- | ---------------------------------------------------------- |
| `GITHUB_TOKEN` (auto)    | all           | ghcr.io push, create releases, open issues                 |
| `GHCR_TOKEN` (optional)  | publish charts | used by `publish-charts.yml` if present; else falls back to `GITHUB_TOKEN` |
| `RAUC_SIGNING_KEY`       | release       | PEM private key for RAUC bundle signing (beta)             |
| `RAUC_SIGNING_CERT`      | release       | PEM certificate paired with the signing key (beta)         |
| `TELEMETRY_API_KEY`      | optional       | forwarded to workflows that emit build telemetry           |

**cosign** is fully keyless — no secrets to manage. GitHub-hosted runners' OIDC identity is what Sigstore's Fulcio CA attests.

---

## GA: migrate RAUC signing to Cloud KMS

The beta release flow stores the RAUC signing key as an Actions secret. Acceptable for public beta; **not acceptable for GA**. Migration path:

1. Provision a signing key in AWS KMS (`Sign` action) / Azure Key Vault / GCP KMS. Never export the private key.
2. Use a PKCS#11 provider (e.g. `aws-kms-pkcs11`, `cloud-kms-pkcs11`) so RAUC can invoke the HSM via `--cert pkcs11:...` and `--key pkcs11:...`.
3. Authenticate the runner to the KMS via GitHub OIDC -> IAM role (AWS) / federated identity (Azure/GCP). Zero long-lived credentials.
4. Replace the secret-backed `hack/ci/rauc-sign-release.sh` call with a KMS-backed one. Remove `RAUC_SIGNING_KEY` / `_CERT` secrets.

A `TODO(GA)` comment in `release.yml` and `hack/ci/rauc-sign-release.sh` anchors this migration.

---

## Local invocation

Most CI commands work locally:

```bash
# PR-style Rust checks (no SPDK linking)
cd storage/dataplane
cargo fmt --check
cargo clippy --no-default-features --all-targets -- -D warnings
cargo build --no-default-features
cargo test --no-default-features

# OS pipeline (needs Linux + root)
make -C os base layered bundle iso

# App charts
./apps/scripts/publish-all.sh   # requires helm + ghcr login
./apps/scripts/sign-all.sh      # requires cosign

# Perf compare
python3 perf/compare.py --baseline perf/baseline.csv \
  --results perf/results/ --threshold-pct 10 --report /tmp/report.md
```

---

## Troubleshooting

- **actionlint failures** — all workflows are validated by `actionlint`. If you edit YAML, run `actionlint .github/workflows/*.yml` locally (install with `go install github.com/rhysd/actionlint/cmd/actionlint@latest`).
- **kind failures in s3-compat-nightly** — most often a helm `--wait` timeout. Check the `Port-forward S3 endpoint` step's stderr and the uploaded `pf.log` artifact.
- **Perf regression false positive** — rerun the nightly on the branch; if real, lift the 10% threshold in the `workflow_dispatch` inputs once, confirm, then update `perf/baseline.csv` if the regression is justified.
- **RAUC signing "key mismatch"** — `RAUC_SIGNING_CERT` and `RAUC_SIGNING_KEY` must be the matching pair (both PEM). Verify locally: `openssl x509 -in cert.pem -pubkey -noout` equals `openssl pkey -in key.pem -pubout`.
