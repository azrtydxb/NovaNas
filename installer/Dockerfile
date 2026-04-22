# syntax=docker/dockerfile:1.7

# ---- build stage ----
FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
ENV GOWORK=off CGO_ENABLED=0
RUN go build -ldflags="-s -w" -o /out/novanas-installer .

# ---- runtime stage ----
# Not distroless: we need parted, mkfs.*, grub-install, lsblk, unsquashfs, mdadm at runtime.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      util-linux parted dosfstools e2fsprogs mdadm \
      grub-efi-amd64-bin grub-common squashfs-tools \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/novanas-installer /usr/sbin/novanas-installer
ENTRYPOINT ["/usr/sbin/novanas-installer"]
