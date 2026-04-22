# Duplicati Helm Chart

Free backup client with encryption.

## Install

```sh
helm install duplicati oci://ghcr.io/azrtydxb/novanas-apps/duplicati --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://www.duplicati.com
