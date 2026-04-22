# PostgreSQL Helm Chart

Powerful open-source relational database.

## Install

```sh
helm install postgres oci://ghcr.io/azrtydxb/novanas-apps/postgres --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://www.postgresql.org
