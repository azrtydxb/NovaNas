# syntax=docker/dockerfile:1.6
# Multi-stage build for the NovaNas storage agent.
#
# RUNTIME NOTE: the agent drives chunk I/O on local block devices and, for
# NVMe-oF targets, programs kernel nvmet through /sys/kernel/config/nvmet.
# Deployments MUST grant the following Linux capabilities (or run the pod
# as privileged):
#   - SYS_ADMIN  -- mount/umount, configfs writes
#   - NET_ADMIN  -- NVMe-oF transport + listener management
#
# These are intentionally NOT baked into the image; they are granted by
# the Kubernetes DaemonSet security context.

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY storage/ storage/
COPY packages/operators/ packages/operators/
COPY packages/cli/ packages/cli/
WORKDIR /src/storage
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/novanas-storage-agent ./cmd/agent/

FROM debian:bookworm-slim
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates tzdata && \
    rm -rf /var/lib/apt/lists/*
COPY --from=build /out/novanas-storage-agent /usr/local/bin/novanas-storage-agent
USER root
ENTRYPOINT ["/usr/local/bin/novanas-storage-agent"]
