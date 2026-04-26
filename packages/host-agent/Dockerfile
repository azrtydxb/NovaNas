# NovaNas host-agent: a generic config-poller sidecar (#55).
# Polls /api/v1/<resource>, renders a Go text/template, writes the
# result, and reloads the colocated daemon when the rendered output
# changes. See main.go for flags.

FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY packages/sdk/go-client packages/sdk/go-client
COPY packages/host-agent packages/host-agent
COPY packages/operators packages/operators
COPY packages/cli packages/cli
COPY storage storage
WORKDIR /src/packages/host-agent
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/host-agent .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/host-agent /usr/local/bin/host-agent
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/host-agent"]
