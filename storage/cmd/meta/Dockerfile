# syntax=docker/dockerfile:1.6
# Multi-stage build for the NovaNas metadata service.
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY storage/ storage/
COPY packages/operators/ packages/operators/
COPY packages/cli/ packages/cli/
COPY installer/ installer/
WORKDIR /src/storage
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/novanas-storage-meta ./cmd/meta/

FROM debian:bookworm-slim
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates tzdata && \
    rm -rf /var/lib/apt/lists/*
COPY --from=build /out/novanas-storage-meta /usr/local/bin/novanas-storage-meta
ENTRYPOINT ["/usr/local/bin/novanas-storage-meta"]
