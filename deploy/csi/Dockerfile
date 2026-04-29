# syntax=docker/dockerfile:1.7
#
# NovaNAS CSI driver image.
#
# Build context MUST be the repository root so that the Go module
# (with its `replace github.com/novanas/nova-nas/clients/go/novanas =>
# ./clients/go/novanas` directive) resolves the local SDK module.
#
#   docker build -f deploy/csi/Dockerfile -t novanas/csi:dev .
#
# Multi-arch: linux/amd64 only for now. The runtime image is debian-slim
# because we need full mount tooling (mount, blkid, mkfs.ext4/xfs, resize2fs,
# xfs_growfs); a from-scratch or distroless image is impractical.

############################
# Stage 1: build the binary
############################
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /src

# Cache the module graph. The `replace` directive points at
# clients/go/novanas, so we have to copy that submodule's go.mod/go.sum
# *before* `go mod download` will succeed.
COPY go.mod go.sum ./
COPY clients/go/novanas/go.mod clients/go/novanas/go.sum ./clients/go/novanas/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy only what the CSI binary needs to build. Keeping this list narrow
# means edits to unrelated parts of the tree don't bust the layer cache.
COPY cmd/nova-csi ./cmd/nova-csi
COPY internal/csi ./internal/csi
COPY clients/go/novanas ./clients/go/novanas

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags '-s -w' -o /out/nova-csi ./cmd/nova-csi

############################
# Stage 2: runtime image
############################
FROM debian:bookworm-slim

# Mount tooling:
#   mount, umount, blkid    -> util-linux (mount package depends on it)
#   mkfs.ext4, resize2fs    -> e2fsprogs
#   mkfs.xfs, xfs_growfs    -> xfsprogs
#   zfs, zpool (debug only) -> zfsutils-linux (the host owns the pool;
#                              these are kept for diagnostics)
#   ca-certificates         -> required for any TLS to a public endpoint;
#                              also includes /etc/ssl/certs symlinks
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates \
        mount \
        util-linux \
        e2fsprogs \
        xfsprogs \
        zfsutils-linux \
 && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/nova-csi /usr/local/bin/nova-csi

# CSI sockets typically live under /csi (controller) or
# /var/lib/kubelet/plugins/<driver>/csi.sock (node). The DaemonSet/Deployment
# manifests bind-mount the right path; we don't pre-create it here.

ENTRYPOINT ["/usr/local/bin/nova-csi"]
