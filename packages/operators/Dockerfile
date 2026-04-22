# syntax=docker/dockerfile:1.7

# ---- build stage -----------------------------------------------------------
FROM golang:1.23 AS build
WORKDIR /workspace

# Module download first so the layer caches across source edits.
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download || true

COPY . .
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
