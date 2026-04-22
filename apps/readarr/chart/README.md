# Readarr Helm Chart

Book manager and automation tool.

## Install

```sh
helm install readarr oci://ghcr.io/azrtydxb/novanas-apps/readarr --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://readarr.com
