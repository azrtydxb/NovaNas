# Nextcloud Helm Chart

Self-hosted productivity platform for file sync and share.

## Install

```sh
helm install nextcloud oci://ghcr.io/azrtydxb/novanas-apps/nextcloud --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://nextcloud.com
