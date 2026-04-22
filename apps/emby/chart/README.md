# Emby Server Helm Chart

Personal media server with apps on just about every device.

## Install

```sh
helm install emby oci://ghcr.io/azrtydxb/novanas-apps/emby --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://emby.media
