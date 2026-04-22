# MongoDB Helm Chart

Document-oriented NoSQL database.

## Install

```sh
helm install mongodb oci://ghcr.io/azrtydxb/novanas-apps/mongodb --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://www.mongodb.com
