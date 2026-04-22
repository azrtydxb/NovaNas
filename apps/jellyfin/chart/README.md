# Jellyfin Helm Chart

Free software media system, a fork of Emby.

## Install

```sh
helm install jellyfin oci://ghcr.io/azrtydxb/novanas-apps/jellyfin --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://jellyfin.org
