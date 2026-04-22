# Paperless-ngx Helm Chart

Document management system.

## Install

```sh
helm install paperless-ngx oci://ghcr.io/azrtydxb/novanas-apps/paperless-ngx --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://docs.paperless-ngx.com
