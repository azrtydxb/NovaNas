# NovaNas

[![CI](https://github.com/azrtydxb/NovaNas/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/azrtydxb/NovaNas/actions/workflows/ci.yml)
[![storage-spdk](https://github.com/azrtydxb/NovaNas/actions/workflows/storage-spdk.yml/badge.svg?branch=main)](https://github.com/azrtydxb/NovaNas/actions/workflows/storage-spdk.yml)
[![os-build](https://github.com/azrtydxb/NovaNas/actions/workflows/os-build.yml/badge.svg?branch=main)](https://github.com/azrtydxb/NovaNas/actions/workflows/os-build.yml)
[![publish-charts](https://github.com/azrtydxb/NovaNas/actions/workflows/publish-charts.yml/badge.svg?branch=main)](https://github.com/azrtydxb/NovaNas/actions/workflows/publish-charts.yml)
[![s3-compat-nightly](https://github.com/azrtydxb/NovaNas/actions/workflows/s3-compat-nightly.yml/badge.svg)](https://github.com/azrtydxb/NovaNas/actions/workflows/s3-compat-nightly.yml)
[![perf-nightly](https://github.com/azrtydxb/NovaNas/actions/workflows/perf-nightly.yml/badge.svg)](https://github.com/azrtydxb/NovaNas/actions/workflows/perf-nightly.yml)

Developer CI reference: [`docs/developer/ci.md`](docs/developer/ci.md).

## Quick local dev

```bash
make dev-up
# UI:       http://localhost:5173
# API:      http://localhost:8080
# Keycloak: http://localhost:8180
```

See [dev/README.md](dev/README.md) for details (credentials, optional
services, troubleshooting).

## Overview

NovaNas is a Kubernetes-native single-node NAS appliance providing unified block, file, and object storage with integrated container and VM hosting. It is forked from NovaStor (an SDS project) and evolves independently as a focused appliance product.

The appliance targets single-node hardware (QNAP/TrueNAS class and up) and exposes iSCSI, NVMe-oF, NFS, SMB, and S3. It bundles k3s, Keycloak, OpenBao, and the Nova networking stack into one installable image with a single CalVer version number. Users and admins interact with a domain-shaped REST + WebSocket API — Kubernetes is an implementation detail, never exposed in the UI.

## Documentation

All design documentation lives in [`docs/`](docs/). Start with [`docs/README.md`](docs/README.md) for the index.

## Quickstart

Prerequisites: Node.js 22+, pnpm 9+, Go 1.23+, Rust stable.

```sh
pnpm install
pnpm build
```

Additional bootstrap helpers live in [`hack/`](hack/).

## Repository layout

```
novanas/
├── packages/        # TypeScript packages (schemas, api, ui, db, operators, cli, csi)
├── storage/         # Forked NovaStor: chunk engine, metadata, agents, dataplane (Rust)
├── proto/           # gRPC protobuf contracts
├── os/              # Immutable Debian image + RAUC A/B bundle build
├── installer/       # Text-mode installer
├── helm/            # NovaNas umbrella Helm chart
├── apps/            # Official app catalog charts
├── e2e/             # End-to-end tests (Playwright + QEMU)
├── docs/            # Design + user documentation
├── hack/            # Build and developer scripts
└── .github/         # CI workflows
```

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE) for attribution to the upstream NovaStor project.
