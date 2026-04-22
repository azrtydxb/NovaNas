# MariaDB Helm Chart

Community fork of MySQL.

## Install

```sh
helm install mariadb oci://ghcr.io/azrtydxb/novanas-apps/mariadb --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://mariadb.org
