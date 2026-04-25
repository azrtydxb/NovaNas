# syntax=docker/dockerfile:1.7
#
# Build context MUST be the repo root (not packages/operators) so the
# go.work workspace and the storage/ replace target are visible to the
# Go toolchain.

# ---- build stage -----------------------------------------------------------
FROM golang:1.25 AS build
WORKDIR /src

# Pull in the workspace + every module operators directly or transitively
# depends on via replace directives.
COPY go.work go.work.sum* ./
COPY packages/operators packages/operators
COPY packages/cli packages/cli
COPY storage storage

WORKDIR /src/packages/operators
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/manager .

# ---- runtime stage ---------------------------------------------------------
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/manager /manager
USER 65532:65532
EXPOSE 8080 8081
ENTRYPOINT ["/manager"]
