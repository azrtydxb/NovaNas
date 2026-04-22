# Redis Helm Chart

In-memory data structure store.

## Install

```sh
helm install redis oci://ghcr.io/azrtydxb/novanas-apps/redis --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://redis.io
