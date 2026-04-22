# MySQL Helm Chart

Popular open-source relational database.

## Install

```sh
helm install mysql oci://ghcr.io/azrtydxb/novanas-apps/mysql --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://www.mysql.com
