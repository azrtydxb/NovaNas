# syntax=docker/dockerfile:1.6
# Multi-stage build for the NovaNas admission webhook.
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY storage/ storage/
COPY packages/operators/ packages/operators/
COPY packages/cli/ packages/cli/
WORKDIR /src/storage
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/novanas-storage-webhook ./cmd/webhook/

FROM debian:bookworm-slim
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates tzdata && \
    rm -rf /var/lib/apt/lists/*
COPY --from=build /out/novanas-storage-webhook /usr/local/bin/novanas-storage-webhook
ENTRYPOINT ["/usr/local/bin/novanas-storage-webhook"]
