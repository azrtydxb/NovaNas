# syntax=docker/dockerfile:1
FROM golang:1.23 AS builder
WORKDIR /src
COPY . .
ENV CGO_ENABLED=0
RUN go mod download && \
    go build -trimpath -ldflags="-s -w" -o /out/novanasctl .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/novanasctl /usr/local/bin/novanasctl
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/novanasctl"]
