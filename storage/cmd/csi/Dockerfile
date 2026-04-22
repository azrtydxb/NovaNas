# syntax=docker/dockerfile:1.6
# Multi-stage build for the NovaNas CSI driver.
#
# RUNTIME NOTE: the CSI node plugin mounts filesystems into kubelet's
# pod volume paths. It requires SYS_ADMIN on the node-plugin DaemonSet.
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY storage/ storage/
COPY packages/operators/ packages/operators/
COPY packages/cli/ packages/cli/
COPY installer/ installer/
WORKDIR /src/storage
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/novanas-storage-csi ./cmd/csi/

FROM debian:bookworm-slim
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates e2fsprogs xfsprogs util-linux tzdata && \
    rm -rf /var/lib/apt/lists/*
COPY --from=build /out/novanas-storage-csi /usr/local/bin/novanas-storage-csi
ENTRYPOINT ["/usr/local/bin/novanas-storage-csi"]
