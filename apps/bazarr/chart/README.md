# Bazarr Helm Chart

Companion to Sonarr and Radarr for subtitle management.

## Install

```sh
helm install bazarr oci://ghcr.io/azrtydxb/novanas-apps/bazarr --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://www.bazarr.media
