# Lidarr Helm Chart

Looks and smells like Sonarr but made for music.

## Install

```sh
helm install lidarr oci://ghcr.io/azrtydxb/novanas-apps/lidarr --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://lidarr.audio
