# Seafile Helm Chart

Enterprise file sync and share platform.

## Install

```sh
helm install seafile oci://ghcr.io/azrtydxb/novanas-apps/seafile --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://www.seafile.com
