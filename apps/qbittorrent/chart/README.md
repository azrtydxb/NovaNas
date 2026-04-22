# qBittorrent Helm Chart

Open-source BitTorrent client with web UI.

## Install

```sh
helm install qbittorrent oci://ghcr.io/azrtydxb/novanas-apps/qbittorrent --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://www.qbittorrent.org
