# syntax=docker/dockerfile:1.7
#
# disk-agent — node-local DaemonSet that discovers /sys/block and
# upserts Disk CRs. Build context MUST be the repo root (the agent
# lives in the operators module which has go.work replace targets
# back to ./storage).

FROM golang:1.25 AS build
WORKDIR /src

COPY go.work go.work.sum* ./
COPY packages/operators packages/operators
COPY packages/cli packages/cli
COPY storage storage

WORKDIR /src/packages/operators
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/disk-agent ./cmd/disk-agent

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/disk-agent /disk-agent
USER 0:0
# We run as root because we need to read /sys/block (most fields) and
# /dev/disk/by-id symlinks. The DaemonSet drops every capability
# except DAC_READ_SEARCH so the blast radius is minimal even with
# UID 0.
ENTRYPOINT ["/disk-agent"]
