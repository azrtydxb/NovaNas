# syntax=docker/dockerfile:1.6

# Multi-stage Dockerfile for nova-api. Used for CI runs and dev images;
# production deploys via the .deb package onto the host (see deploy/).
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOFLAGS="-trimpath" go build -ldflags="-s -w" -o /out/nova-api ./cmd/nova-api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/nova-api /usr/local/bin/nova-api
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/nova-api"]
