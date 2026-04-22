# Loki Helm Chart

Horizontally-scalable log aggregation system.

## Install

```sh
helm install loki oci://ghcr.io/azrtydxb/novanas-apps/loki --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://grafana.com/oss/loki
