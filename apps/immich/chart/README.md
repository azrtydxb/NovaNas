# Immich Helm Chart

Self-hosted photo and video backup with ML.

## Install

```sh
helm install immich oci://ghcr.io/azrtydxb/novanas-apps/immich --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://immich.app
