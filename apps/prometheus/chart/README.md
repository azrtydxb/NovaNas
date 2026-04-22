# Prometheus Helm Chart

Monitoring system and time-series database.

## Install

```sh
helm install prometheus oci://ghcr.io/azrtydxb/novanas-apps/prometheus --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://prometheus.io
