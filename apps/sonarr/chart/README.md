# Sonarr Helm Chart

Smart PVR for newsgroup and bittorrent users.

## Install

```sh
helm install sonarr oci://ghcr.io/azrtydxb/novanas-apps/sonarr --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://sonarr.tv
