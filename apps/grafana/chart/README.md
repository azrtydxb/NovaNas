# Grafana Helm Chart

Open observability platform.

## Install

```sh
helm install grafana oci://ghcr.io/azrtydxb/novanas-apps/grafana --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://grafana.com
